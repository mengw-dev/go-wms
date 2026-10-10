package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
)

// 收货业务。

// receiveIdempotencyScope 收货命令的幂等作用域（与拣货/上架/盘点审核区分）。
const receiveIdempotencyScope = "inbound.receive"

// Receive 收货：qty 是本次收货总量（包含残品），与前端及 received_qty 的含义一致。
// idempotencyKey 非空时启用请求级幂等：同 key + 同内容重试回放空成功（data:null 契约不变），
// 同 key 不同内容返回 409；未携带 key 保持旧契约但无请求级去重保证。
func (s *Service) Receive(ctx context.Context, orderID, detailID int64, req *dto.ReceiveReq, operator, idempotencyKey string) error {
	// qty 是本次收货总量（包含残品），与前端及 received_qty 的含义一致。
	if req.Qty <= 0 || req.DefectiveQty < 0 || req.DefectiveQty > req.Qty {
		return errcode.ParamError
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 64 {
		return errcode.ParamError
	}
	tenantID := tenant.FromContext(ctx)
	var requestHash string
	if idempotencyKey != "" {
		// 批次按既有业务口径参与指纹（精确字符串比较，不含空格/大小写归一化），不改变收货语义。
		requestHash = receiveRequestHash(orderID, detailID, req)
	}
	err := s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		// 幂等快路径：同 key 重试回放空成功；同 key 不同内容属于客户端误用，不可重试。
		if idempotencyKey != "" {
			record, err := idempotency.Find(tx, tenantID, receiveIdempotencyScope, idempotencyKey)
			if err != nil {
				return err
			}
			if record != nil {
				if record.RequestHash != requestHash {
					return errcode.IdempotencyKeyReused
				}
				// 回放此前成功提交的空成功标记；标记缺失/损坏按内部错误处理，
				// 不能用「当前单据进度」伪装成功。
				return idempotency.ValidateEmptySuccess(record.ResultJSON)
			}
		}
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		if o.Status != model.OrderApproved && o.Status != model.OrderReceiving {
			return errcode.OrderStatusWrong
		}
		d, err := s.repo.GetDetailForUpdate(tx, detailID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		if d.OrderID != orderID {
			return errcode.ParamError
		}
		remaining := d.ExpectedQty - d.ReceivedQty
		if req.Qty > remaining {
			return errcode.ReceiveQtyOver
		}
		// 批次号：首次收货必填并落库；后续保持一致
		if d.BatchNo == "" {
			if req.BatchNo == "" {
				return errcode.BatchNoRequired
			}
			d.BatchNo = req.BatchNo
		} else if req.BatchNo != "" && req.BatchNo != d.BatchNo {
			return errcode.BatchNoInconsistent
		}
		// 明细原子累加（批次号仅首次落库）
		if err := s.repo.IncrDetailReceive(tx, d, req.Qty, req.DefectiveQty); err != nil {
			return err
		}
		// 收货任务推进：qty 已包含良品和残品，残品不能重复计数。
		// 收齐时任务完成量恰好等于目标量，任务自动 CREATED → IN_PROGRESS → COMPLETED。
		if err := s.taskAPI.AddProgressByDetail(ctx, tx, orderID, detailID, taskmodel.TaskReceive, req.Qty, operator); err != nil {
			return err
		}

		// 重读全部明细（同事务可见原子累加后的新值）判断是否全部收齐
		all, err := s.repo.ListDetails(tx, orderID)
		if err != nil {
			return err
		}
		fullyReceived := true
		for _, item := range all {
			if item.ReceivedQty < item.ExpectedQty {
				fullyReceived = false
				break
			}
		}
		var toStatus model.OrderStatus
		var putawayTasks []*taskapi.CreateTask
		switch {
		case fullyReceived:
			// 上架任务（残品不入库，上架量 = 已收 - 残品）
			for _, item := range all {
				putawayQty := item.ReceivedQty - item.DefectiveQty
				if putawayQty <= 0 {
					continue
				}
				putawayTasks = append(putawayTasks, &taskapi.CreateTask{
					TaskType: taskmodel.TaskPutaway, OrderID: o.ID, OrderNo: o.OrderNo,
					DetailID: item.ID, SKUID: item.SKUID, WarehouseID: o.WarehouseID, TargetQty: putawayQty,
				})
			}
			if len(putawayTasks) > 0 {
				toStatus = model.OrderPutaway
			} else {
				// 全部残品：无上架作业，收齐即完成（否则单据永远停在 PUTAWAY）
				toStatus = model.OrderCompleted
			}
			// 状态机校验：APPROVED（首次收货即收齐）或 RECEIVING → PUTAWAY/COMPLETED
			if !model.CanTransit(o.Status, toStatus) {
				return errcode.OrderStatusWrong
			}
		case o.Status == model.OrderApproved:
			// 状态机校验：首次部分收货 APPROVED → RECEIVING
			if !model.CanTransit(o.Status, model.OrderReceiving) {
				return errcode.OrderStatusWrong
			}
			toStatus = model.OrderReceiving
		default:
			toStatus = o.Status // RECEIVING 中继续收货，状态不变
		}
		// 主单原子累加 + 状态推进（version 乐观锁，冲突由 TxRetry 重试）
		if n, err := s.repo.IncrOrderReceive(tx, o.ID, o.Version, req.Qty, req.DefectiveQty, toStatus); err != nil {
			return err
		} else if n == 0 {
			return errcode.OrderVersionBad
		}
		// 收齐 → 生成上架任务
		if len(putawayTasks) > 0 {
			if err := s.taskAPI.Create(ctx, tx, putawayTasks); err != nil {
				return err
			}
		}
		// 幂等记录与业务写入同事务提交：业务失败随事务回滚，key 可复用。
		if idempotencyKey != "" {
			record := &idempotency.Record{
				ID: snowflake.Next(), TenantID: tenantID,
				Scope: receiveIdempotencyScope, IdempotencyKey: idempotencyKey,
				RequestHash: requestHash, ObjectID: orderID,
				ResultJSON: idempotency.EmptySuccessJSON,
			}
			if err := idempotency.Insert(tx, record); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		// 业务写入已随事务回滚；同 key 并发下「后到者」可能因快照读不到先到者
		// 已提交的记录，拿锁后又因旧状态（超量/状态变化）被拒绝：改用事务外新读核对，
		// 命中则回放空成功或给出准确 409，不把后到者误报为新操作失败。
		if idempotencyKey != "" {
			if reconcileErr, handled := s.reconcileReceive(ctx, tenantID, idempotencyKey, requestHash); handled {
				return reconcileErr
			}
		}
		return err
	}
	return nil
}

// receiveRequestHash 收货指纹：单据 ID、明细 ID、收货量、残品量与批次号。
// 批次号按业务既有口径（精确比较）参与，不做 trim/大小写归一化，避免改变收货语义；
// 时间戳、操作人等每次请求可变的字段不参与。
func receiveRequestHash(orderID, detailID int64, req *dto.ReceiveReq) string {
	return idempotency.Fingerprint(
		strconv.FormatInt(orderID, 10),
		strconv.FormatInt(detailID, 10),
		strconv.Itoa(req.Qty),
		strconv.Itoa(req.DefectiveQty),
		req.BatchNo,
	)
}

// reconcileReceive 业务事务回滚后，用新读核对已提交的同 key 幂等记录（语义与拣货一致）：
// 命中且指纹一致且空成功标记合法 → 回放成功；指纹不同 → 409；记录损坏 → 内部错误；
// 未命中/读取失败/请求已取消 → handled=false，保留原业务错误。
func (s *Service) reconcileReceive(ctx context.Context, tenantID int64, idempotencyKey, requestHash string) (error, bool) {
	if cancelErr := ctx.Err(); cancelErr != nil {
		// 请求已取消：不发起核对查询；handled=false，调用方保留原业务错误。
		return cancelErr, false
	}
	record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, receiveIdempotencyScope, idempotencyKey)
	if err != nil {
		return err, false
	}
	if record == nil {
		return nil, false
	}
	if record.RequestHash != requestHash {
		return errcode.IdempotencyKeyReused, true
	}
	if err := idempotency.ValidateEmptySuccess(record.ResultJSON); err != nil {
		return err, true
	}
	return nil, true
}
