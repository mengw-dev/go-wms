package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
)

// 拣货和发货扣减。

// pickLeaseTTL 拣货任务租约时长：领取后超过该时长未操作，其他 PDA 可以接手。
const pickLeaseTTL = 10 * time.Minute

// PickScan 拣货前的扫码核对信息，为空字段表示该维度不校验。
// Strict 为 true 时用于 PDA 入口：库位必填且必须一致，任务有批次时批次必填且必须一致。
type PickScan struct {
	LocationCode string
	BatchNo      string
	Strict       bool
}

// pickIdempotencyScope 拣货命令的幂等作用域（与收货/上架/盘点审核区分）。
const pickIdempotencyScope = "outbound.pick"

// claimIdempotencyScope 领取命令的幂等作用域：重试同一领取请求回放同一凭证，不轮换 token。
const claimIdempotencyScope = "outbound.claim"

// Pick 按分配行拣货，返回提交时刻（幂等命中时为首次成功）的任务快照。
// scan 非空时校验扫描的库位/批次与任务一致，Strict 模式（PDA）下缺失也拒绝；
// claimToken 非空时校验任务领取凭证与租约；idempotencyKey 非空时启用请求级幂等，
// 重试命中优先回放首次成功的结果快照（在凭证校验之前短路）；
// 指纹由任务、数量、规范化后的扫码内容与入口类型（PDA/后台）共同决定。
//
// 业务拒绝（数量超限/状态不允许/关系不一致等）时事务已回滚，
// 返回非锁定读取的当前快照与错误，供 PDA 就地刷新进度。
//
// 锁协议（与取消统一）：任务行 → 分配行 → 库存行 → 主单行。
// 不再先锁整张出库单：同一张单的不同任务可以并行拣货；
// 与取消的互斥落在任务行上——取消会先锁定全部任务行并校验是否已有任务开工。
func (s *Service) Pick(ctx context.Context, taskID int64, qty int, operator string, scan *PickScan, claimToken, idempotencyKey string) (*dto.PickResult, error) {
	// 事务外只读不可变路由信息（OrderID/AllocationID/TaskType/TaskNo 建后不变）
	routing, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if routing.TaskType != taskmodel.TaskPick {
		return nil, errcode.TaskStatusWrong
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 64 {
		return nil, errcode.ParamError
	}
	tenantID := tenant.FromContext(ctx)
	var requestHash string
	if idempotencyKey != "" {
		requestHash = pickRequestHash(taskID, qty, scan)
	}
	var result *dto.PickResult
	err = s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		// 每轮重试重新计算结果快照，避免失败轮次的内存状态残留
		result = nil
		// 幂等快路径：同 key + 同内容回放首次成功的快照；
		// 同 key + 不同内容属于客户端误用，不可重试。
		if idempotencyKey != "" {
			record, err := idempotency.Find(tx, tenantID, pickIdempotencyScope, idempotencyKey)
			if err != nil {
				return err
			}
			if record != nil {
				if record.RequestHash != requestHash {
					return errcode.IdempotencyKeyReused
				}
				// 回放首次成功快照；快照缺失或损坏按内部错误处理，不用当前进度兜底
				replayed, err := replayPickResult(record.ResultJSON)
				if err != nil {
					return err
				}
				result = replayed
				return nil
			}
		}
		// 第一把锁：任务行。全链路最细的互斥点——取消同单任务、推进任务进度
		// 都必须先拿到它；任务已被取消则在这里直接拒绝，不再依赖主单锁。
		t, err := s.taskAPI.GetForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if t.Status != taskmodel.TaskCreated && t.Status != taskmodel.TaskInProgress {
			return errcode.TaskStatusWrong
		}
		// 领取凭证校验：PDA 提交必须持有效凭证；后台入口不传凭证则保持宽松
		if err := checkPickClaim(claimToken, t); err != nil {
			return err
		}
		// 第二把锁：分配行
		a, err := s.repo.GetAllocationForUpdate(tx, routing.AllocationID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.ShipOrderNotFound
			}
			return err
		}
		if a.Status != model.AllocAllocated {
			return errcode.TaskStatusWrong
		}
		// 聚合关系校验（分层）：拣货内只校验任务 ↔ 分配行这几条边；
		// 明细、库存等其他边的正确性由各自的锁读与条件更新继续约束。
		if t.AllocationID != a.ID ||
			a.OrderID != t.OrderID ||
			a.SKUID != t.SKUID ||
			a.AllocatedQty != t.TargetQty ||
			a.DetailID != t.DetailID {
			return errcode.TaskAllocationMismatch
		}
		if err := checkPickScan(scan, t); err != nil {
			return err
		}
		// 推进拣货任务（复用已锁定的任务行；校验数量不超剩余 + 任务状态机）
		if err := s.taskAPI.AddProgress(ctx, tx, t, qty, operator); err != nil {
			return err
		}
		// 分配行原子累加；拣满置 PICKED（行已锁，base+delta 即更新后值，用于决策）
		newAllocPicked := a.PickedQty + qty
		allocToStatus := model.AllocAllocated
		allocFullyPicked := newAllocPicked == a.AllocatedQty
		if allocFullyPicked {
			allocToStatus = model.AllocPicked
		}
		if n, err := s.repo.IncrAllocationPicked(tx, a, qty, allocToStatus); err != nil {
			return err
		} else if n == 0 {
			return errcode.AllocConflict
		}
		// 分配行拣满 → 发货扣减库存（先库存后主单，保持全局加锁顺序）
		if allocFullyPicked {
			if err := s.inv.Ship(ctx, tx, &invapi.ShipReq{
				InventoryID: a.InventoryID, Quantity: a.AllocatedQty,
				OrderNo: t.OrderNo, TaskNo: t.TaskNo, Operator: operator,
			}); err != nil {
				return err
			}
		}
		// 主单原子累加：纯相对更新，不再依赖锁读出的旧版本；
		// 状态条件防止把已取消/已发货的主单继续累加。
		if n, err := s.repo.IncrOrderPicked(tx, t.OrderID, qty); err != nil {
			return err
		} else if n == 0 {
			return errcode.ShipOrderStatusWrong
		}
		// 明细原子累加（SQL 带 picked_qty + qty <= allocated_qty 上限）；
		// 0 行说明明细不存在或数量关系已错乱，停止拣货并整体回滚。
		if n, err := s.repo.IncrDetailPicked(tx, a.DetailID, qty); err != nil {
			return err
		} else if n == 0 {
			return errcode.TaskAllocationMismatch
		}

		// 全部拣完 → SHIPPED：完成判定下沉到 SQL（picked_qty = allocated_qty），
		// 返回 0 表示本次拣货没有完成整单，属正常情况，不报错。
		orderStatus := model.OrderPicking
		if n, err := s.repo.ShipIfFullyPicked(tx, t.OrderID); err != nil {
			return err
		} else if n > 0 {
			orderStatus = model.OrderShipped
		}
		// 持有人续租：拣货成功后延长租约，避免作业中途被他人接手
		if claimToken != "" {
			if _, err := s.taskAPI.RenewClaim(ctx, tx, taskID, claimToken, time.Now().Add(pickLeaseTTL)); err != nil {
				return err
			}
		}
		// 提交时刻快照：AddProgress 已在内存中推进任务状态与完成量
		result = &dto.PickResult{
			TaskStatus:   t.Status,
			DoneQty:      t.DoneQty,
			RemainingQty: t.TargetQty - t.DoneQty,
			OrderStatus:  orderStatus,
		}
		// 幂等记录与业务写入同事务提交；业务失败时随事务回滚，key 可复用。
		if idempotencyKey != "" {
			payload, err := json.Marshal(result)
			if err != nil {
				return err
			}
			record := &idempotency.Record{
				ID: snowflake.Next(), TenantID: tenantID,
				Scope: pickIdempotencyScope, IdempotencyKey: idempotencyKey,
				RequestHash: requestHash, ObjectID: taskID,
				ResultJSON: string(payload),
			}
			if err := idempotency.Insert(tx, record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// 业务写入已随事务回滚；同 key 并发下「后到者」可能因快照读不到先到者
		// 已提交的记录，拿锁后又因旧状态（任务已完成/剩余量变化）被业务拒绝。
		// 用事务外新读核对已提交记录：命中则回放首次成功或给出准确 409，
		// 不把后到者误报为新业务操作失败（避免前端丢 key 换发造成重复执行）。
		if idempotencyKey != "" {
			replayed, reconcileErr, handled := s.reconcilePick(ctx, tenantID, idempotencyKey, requestHash)
			if handled {
				return replayed, reconcileErr
			}
		}
		// 业务拒绝：事务已回滚，补一次非锁定读取返回当前进度（读取失败时快照为 nil）
		snapshot, _ := s.pickSnapshot(ctx, taskID)
		return snapshot, err
	}
	if result == nil {
		// 理论不可达：业务成功与幂等回放两条路径都会产出快照，防御性返回内部错误
		return nil, errcode.Internal
	}
	return result, nil
}

// ClaimPickTask 领取（或本人续领）拣货任务，返回领取凭证、租约到期时间与任务快照。
// idempotencyKey 非空时启用请求级幂等：同一 key + 同一内容（任务+操作人）回放首次领取的
// 凭证与租约，不轮换 token；同 key 不同内容返回 409。PDA 入口强制携带（见 handler）。
func (s *Service) ClaimPickTask(ctx context.Context, taskID int64, operator, idempotencyKey string) (*dto.ClaimResult, error) {
	if operator == "" {
		return nil, errcode.ParamError
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 64 {
		return nil, errcode.ParamError
	}
	tenantID := tenant.FromContext(ctx)
	var requestHash string
	if idempotencyKey != "" {
		requestHash = idempotency.Fingerprint(strconv.FormatInt(taskID, 10), operator)
	}
	var result *dto.ClaimResult
	err := s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		result = nil
		// 幂等快路径：重放同一领取请求，凭证与租约以首次成功为准
		if idempotencyKey != "" {
			record, err := idempotency.Find(tx, tenantID, claimIdempotencyScope, idempotencyKey)
			if err != nil {
				return err
			}
			if record != nil {
				if record.RequestHash != requestHash {
					return errcode.IdempotencyKeyReused
				}
				replayed, err := replayClaimResult(record.ResultJSON)
				if err != nil {
					return err
				}
				result = replayed
				return nil
			}
		}
		token := strconv.FormatInt(snowflake.Next(), 10)
		expireAt := time.Now().Add(pickLeaseTTL)
		n, err := s.taskAPI.Claim(ctx, tx, taskID, operator, token, expireAt)
		if err != nil {
			return err
		}
		if n == 0 {
			return s.claimFailure(ctx, taskID)
		}
		// 快照与领取写入同事务读取：任务行已被本事务的领取 UPDATE 锁定，复用锁定读不引入新等待
		snapshot, err := s.pickSnapshotTx(ctx, tx, taskID)
		if err != nil {
			return err
		}
		result = &dto.ClaimResult{ClaimToken: token, LeaseExpireAt: expireAt, PickResult: *snapshot}
		// 幂等记录与领取写入同事务提交；失败随事务回滚，key 可复用
		if idempotencyKey != "" {
			payload, err := json.Marshal(result)
			if err != nil {
				return err
			}
			record := &idempotency.Record{
				ID: snowflake.Next(), TenantID: tenantID,
				Scope: claimIdempotencyScope, IdempotencyKey: idempotencyKey,
				RequestHash: requestHash, ObjectID: taskID,
				ResultJSON: string(payload),
			}
			if err := idempotency.Insert(tx, record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// 同 key 并发下后到者的领取可能因竞争旧状态被拒：用新读核对已提交记录，
		// 命中则回放首次凭证（或给出准确 409），不把后到者误报为领取失败。
		if idempotencyKey != "" {
			replayed, reconcileErr, handled := s.reconcileClaim(ctx, tenantID, idempotencyKey, requestHash)
			if handled {
				return replayed, reconcileErr
			}
		}
		return nil, err
	}
	return result, nil
}

// reconcilePick 业务事务回滚后，用新读核对已提交的同 key 幂等记录：
//   - 未命中或读取失败：返回 handled=false，保留原业务错误；
//   - 命中且指纹一致：回放首次成功结果；
//   - 命中但指纹不同：409 内容冲突（比状态/数量类错误更准确，前端不换 key 重发）；
//   - 记录损坏：内部错误，不用当前进度伪装成功。
//
// 新读使用事务外连接（每次查询建立新快照），既不复用已回滚事务，
// 也不用旧快照证明「无记录」；请求已取消时直接放弃核对，不发起额外查询。
func (s *Service) reconcilePick(ctx context.Context, tenantID int64, idempotencyKey, requestHash string) (*dto.PickResult, error, bool) {
	if cancelErr := ctx.Err(); cancelErr != nil {
		// 请求已取消：不发起核对查询；handled=false，调用方保留原业务错误。
		return nil, cancelErr, false
	}
	record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, pickIdempotencyScope, idempotencyKey)
	if err != nil {
		return nil, err, false
	}
	if record == nil {
		return nil, nil, false
	}
	if record.RequestHash != requestHash {
		return nil, errcode.IdempotencyKeyReused, true
	}
	replayed, err := replayPickResult(record.ResultJSON)
	if err != nil {
		return nil, err, true
	}
	return replayed, nil, true
}

// reconcileClaim 领取命令的延迟核对，语义与 reconcilePick 一致。
func (s *Service) reconcileClaim(ctx context.Context, tenantID int64, idempotencyKey, requestHash string) (*dto.ClaimResult, error, bool) {
	if cancelErr := ctx.Err(); cancelErr != nil {
		// 请求已取消：不发起核对查询；handled=false，调用方保留原业务错误。
		return nil, cancelErr, false
	}
	record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, claimIdempotencyScope, idempotencyKey)
	if err != nil {
		return nil, err, false
	}
	if record == nil {
		return nil, nil, false
	}
	if record.RequestHash != requestHash {
		return nil, errcode.IdempotencyKeyReused, true
	}
	replayed, err := replayClaimResult(record.ResultJSON)
	if err != nil {
		return nil, err, true
	}
	return replayed, nil, true
}

// claimFailure 领取条件更新 0 行后定位原因：任务不存在/类型或状态不允许，
// 否则为已被他人领取且租约未过期。
func (s *Service) claimFailure(ctx context.Context, taskID int64) error {
	t, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if t.TaskType != taskmodel.TaskPick ||
		(t.Status != taskmodel.TaskCreated && t.Status != taskmodel.TaskInProgress) {
		return errcode.TaskStatusWrong
	}
	return errcode.TaskClaimConflict
}

// checkPickScan 核对拣货员扫描的库位和批次是否与任务要求一致（忽略大小写和首尾空格）。
// Strict 模式（PDA）下缺失也拒绝：库位必须扫描；任务有批次时批次必须扫描。
func checkPickScan(scan *PickScan, t *taskmodel.Task) error {
	if scan == nil {
		return nil
	}
	code := strings.TrimSpace(scan.LocationCode)
	if code == "" {
		if scan.Strict {
			return errcode.PickLocationRequired
		}
	} else if !strings.EqualFold(code, t.LocationCode) {
		return errcode.PickLocationMismatch
	}
	batch := strings.TrimSpace(scan.BatchNo)
	if batch == "" {
		if scan.Strict && t.BatchNo != "" {
			return errcode.PickBatchRequired
		}
	} else if !strings.EqualFold(batch, t.BatchNo) {
		return errcode.PickBatchMismatch
	}
	return nil
}

// checkPickClaim 校验 PDA 领取凭证：凭证不符拒绝；租约已过期拒绝（需重新领取）。
// claimToken 为空（后台入口）时不校验，保持后台宽松语义。
func checkPickClaim(claimToken string, t *taskmodel.Task) error {
	if claimToken == "" {
		return nil
	}
	if t.ClaimToken == "" || t.ClaimToken != claimToken {
		return errcode.TaskClaimMismatch
	}
	if t.LeaseExpireAt != nil && t.LeaseExpireAt.Before(time.Now()) {
		return errcode.TaskLeaseExpired
	}
	return nil
}

// pickSnapshot 非锁定读取任务与主单的当前进度。
func (s *Service) pickSnapshot(ctx context.Context, taskID int64) (*dto.PickResult, error) {
	t, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	result := &dto.PickResult{
		TaskStatus:   t.Status,
		DoneQty:      t.DoneQty,
		RemainingQty: t.TargetQty - t.DoneQty,
	}
	if o, err := s.repo.GetOrder(ctx, s.tm.DB(), t.OrderID); err == nil {
		result.OrderStatus = o.Status
	}
	return result, nil
}

// pickSnapshotTx 事务内读取任务与主单的当前进度（领取事务生成响应快照用）。
// 任务行已被本事务的领取 UPDATE 锁定，复用锁定读不引入新的锁等待，
// 同时保证快照与事务内写入一致，不读事务外的旧数据；回滚后的错误快照仍走非锁定读。
func (s *Service) pickSnapshotTx(ctx context.Context, tx *gorm.DB, taskID int64) (*dto.PickResult, error) {
	t, err := s.taskAPI.GetForUpdate(ctx, tx, taskID)
	if err != nil {
		return nil, err
	}
	result := &dto.PickResult{
		TaskStatus:   t.Status,
		DoneQty:      t.DoneQty,
		RemainingQty: t.TargetQty - t.DoneQty,
	}
	if o, err := s.repo.GetOrder(ctx, tx, t.OrderID); err == nil {
		result.OrderStatus = o.Status
	}
	return result, nil
}

// pickRequestHash 计算拣货请求指纹：任务、数量、扫码内容与入口类型。
// 扫码内容按实际校验口径规范化（去首尾空格 + 大小写折叠），同一次扫描不因大小写差异被误判为新请求；
// 入口类型区分 PDA 严格入口与后台宽松入口；claim_token 不参与指纹——
// 租约过期换人接手后重放同一请求，仍应回放首次成功结果。
func pickRequestHash(taskID int64, qty int, scan *PickScan) string {
	entry, location, batch := "backend", "", ""
	if scan != nil {
		if scan.Strict {
			entry = "pda"
		}
		location = strings.ToLower(strings.TrimSpace(scan.LocationCode))
		batch = strings.ToLower(strings.TrimSpace(scan.BatchNo))
	}
	return idempotency.Fingerprint(strconv.FormatInt(taskID, 10), strconv.Itoa(qty), location, batch, entry)
}

// replayClaimResult 反序列化幂等记录中的首次领取结果（凭证 + 租约 + 快照）。
// 「能解析成 JSON」不等于「有效成功结果」：空串、非法 JSON、null、缺字段或
// 字段类型错误都视为损坏记录，返回内部错误，不把不可重放的记录伪装成成功回放；
// 凭证缺失或租约信息不可解析同样拒绝。租约是否已过期不影响回放（重放返回首次租约）。
func replayClaimResult(raw string) (*dto.ClaimResult, error) {
	fields, err := decodeResultFields(raw)
	if err != nil {
		return nil, err
	}
	if err := requireJSONString(fields, "claim_token"); err != nil {
		return nil, err
	}
	if err := requireJSONString(fields, "lease_expire_at"); err != nil {
		return nil, err
	}
	if err := requirePickSnapshotFields(fields); err != nil {
		return nil, err
	}
	var result dto.ClaimResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, errcode.Wrap(err, errcode.Internal)
	}
	if result.ClaimToken == "" || result.LeaseExpireAt.IsZero() {
		return nil, errcode.Internal
	}
	return &result, nil
}

// replayPickResult 反序列化幂等记录中的首次成功快照；校验语义与 replayClaimResult
// 一致，数量 0 是合法值，不用零值判断缺失。
func replayPickResult(raw string) (*dto.PickResult, error) {
	fields, err := decodeResultFields(raw)
	if err != nil {
		return nil, err
	}
	if err := requirePickSnapshotFields(fields); err != nil {
		return nil, err
	}
	var result dto.PickResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, errcode.Wrap(err, errcode.Internal)
	}
	return &result, nil
}

// requirePickSnapshotFields 校验快照必需字段：状态为字符串、数量为 JSON 数字。
func requirePickSnapshotFields(fields map[string]json.RawMessage) error {
	for _, key := range []string{"task_status", "order_status"} {
		if err := requireJSONString(fields, key); err != nil {
			return err
		}
	}
	for _, key := range []string{"done_qty", "remaining_qty"} {
		if err := requireJSONNumber(fields, key); err != nil {
			return err
		}
	}
	return nil
}

// decodeResultFields 解析回放 JSON 的顶层字段：null / 非法 JSON / 非对象均视为损坏。
func decodeResultFields(raw string) (map[string]json.RawMessage, error) {
	if raw == "" {
		return nil, errcode.Internal
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, errcode.Wrap(err, errcode.Internal)
	}
	if fields == nil {
		return nil, errcode.Internal
	}
	return fields, nil
}

// requireJSONString 校验字段存在且为 JSON 字符串（null / 数字 / 布尔均拒绝）。
func requireJSONString(fields map[string]json.RawMessage, key string) error {
	raw, ok := fields[key]
	if !ok || strings.TrimSpace(string(raw)) == "null" {
		return errcode.Internal
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return errcode.Internal
	}
	return nil
}

// requireJSONNumber 校验字段存在且为 JSON 数字（null / 字符串 / 布尔均拒绝）。
func requireJSONNumber(fields map[string]json.RawMessage, key string) error {
	raw, ok := fields[key]
	if !ok {
		return errcode.Internal
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return errcode.Internal
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return errcode.Internal
	}
	return nil
}
