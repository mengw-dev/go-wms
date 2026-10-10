package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inbound/model"
	invapi "gowms/internal/modules/inventory/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
)

// 上架业务。

// putawayIdempotencyScope 上架命令的幂等作用域（与收货/拣货/盘点审核区分）。
const putawayIdempotencyScope = "inbound.putaway"

// Putaway 上架：库存生效（Increase + RECEIVE 流水）与库位占用、任务推进、单据状态同事务原子提交。
// idempotencyKey 非空时启用请求级幂等：同 key + 同内容重试回放空成功（data:null 契约不变），
// 同 key 不同内容返回 409；未携带 key 保持旧契约但无请求级去重保证。
func (s *Service) Putaway(ctx context.Context, taskID, locationID int64, qty int, operator, idempotencyKey string) error {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 64 {
		return errcode.ParamError
	}
	tenantID := tenant.FromContext(ctx)
	var requestHash string
	if idempotencyKey != "" {
		requestHash = putawayRequestHash(taskID, locationID, qty)
	}
	// 事务外只读不可变路由信息（OrderID/DetailID/SKUID/TaskType/TaskNo/仓库 建后不变）
	routing, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if routing.TaskType != taskmodel.TaskPutaway {
		return errcode.TaskStatusWrong
	}
	// 幂等快路径提前到可变业务状态校验之前：库位可用性会随时间变化（禁用/占用），
	// 已提交成功的同 key 重试必须回放成功，不能被库位当前状态拦截；
	// 库位校验只针对新操作。（身份/租户类访问校验由入口完成，路由读取不受业务状态影响。）
	if idempotencyKey != "" {
		record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, putawayIdempotencyScope, idempotencyKey)
		if err != nil {
			return err
		}
		if record != nil {
			if record.RequestHash != requestHash {
				return errcode.IdempotencyKeyReused
			}
			return idempotency.ValidateEmptySuccess(record.ResultJSON)
		}
	}
	// 库位校验：存在、非禁用，且必须属于单据仓库，防止跨仓库上架产生不一致库存
	if err := s.basic.ValidateLocationInWarehouse(ctx, routing.WarehouseID, locationID); err != nil {
		return err
	}
	err = s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		// 事务内不再做幂等查询：并发同 key 撞唯一键后由 TxRetry 重跑业务，被终态拒绝后
		// 经 reconcilePutaway 新读核对回放。事务第一条语句保持 FOR UPDATE（订单行锁），
		// 完成判定的读视图在订单行锁之后才固定——同单并发上架的提交必被后到者看到，
		// 不会出现全部任务已完成而单据停在 PUTAWAY。
		o, err := s.repo.GetOrderForUpdate(tx, routing.OrderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		if o.Status != model.OrderPutaway {
			return errcode.OrderStatusWrong
		}
		// 事务内行锁读取任务：权威校验任务未被并发取消
		t, err := s.taskAPI.GetForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if t.Status != taskmodel.TaskCreated && t.Status != taskmodel.TaskInProgress {
			return errcode.TaskStatusWrong
		}
		var detail *model.ReceiptOrderDetail
		details, err := s.repo.ListDetails(tx, o.ID)
		if err != nil {
			return err
		}
		for _, d := range details {
			if d.ID == routing.DetailID {
				detail = d
				break
			}
		}
		if detail == nil || detail.BatchNo == "" {
			return errcode.BatchNoRequired
		}
		// 库存生效：上架时才增加库存
		if err := s.inv.Increase(ctx, tx, &invapi.IncreaseReq{
			WarehouseID: o.WarehouseID, LocationID: locationID, SKUID: routing.SKUID,
			BatchNo: detail.BatchNo, Quantity: qty,
			OrderNo: o.OrderNo, TaskNo: routing.TaskNo, Operator: operator,
		}); err != nil {
			return err
		}
		// 库位标记占用
		if err := s.basic.UpdateLocationStatusInTx(ctx, tx, locationID, basicmodel.LocationStatusOccupied); err != nil {
			return err
		}
		// 推进任务（复用已锁定的任务行，含状态机与数量校验）
		if err := s.taskAPI.AddProgress(ctx, tx, t, qty, operator); err != nil {
			return err
		}
		// 全部上架任务完成 → 单据 COMPLETED
		unfinished, err := s.taskAPI.CountUnfinished(ctx, tx, o.ID, taskmodel.TaskPutaway)
		if err != nil {
			return err
		}
		if unfinished == 0 {
			// 状态机校验：PUTAWAY → COMPLETED
			if !model.CanTransit(o.Status, model.OrderCompleted) {
				return errcode.OrderStatusWrong
			}
			if n, err := s.repo.UpdateStatus(tx, o.ID, model.OrderPutaway, model.OrderCompleted); err != nil {
				return err
			} else if n == 0 {
				return errcode.OrderVersionBad
			}
		}
		// 幂等记录与业务写入同事务提交：业务失败随事务回滚，key 可复用。
		if idempotencyKey != "" {
			record := &idempotency.Record{
				ID: snowflake.Next(), TenantID: tenantID,
				Scope: putawayIdempotencyScope, IdempotencyKey: idempotencyKey,
				RequestHash: requestHash, ObjectID: taskID,
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
		// 已提交的记录，拿锁后又因旧状态被拒：改用事务外新读核对，不把后到者误报为新操作失败。
		if idempotencyKey != "" {
			if reconcileErr, handled := s.reconcilePutaway(ctx, tenantID, idempotencyKey, requestHash); handled {
				return reconcileErr
			}
		}
		return err
	}
	return nil
}

// putawayRequestHash 上架指纹：任务 ID、库位 ID 与上架数量。
// 批号由任务↔明细绑定决定、单据与明细归属由任务路由决定，均不重复参与；
// 时间戳、操作人等每次请求可变的字段不参与。
func putawayRequestHash(taskID, locationID int64, qty int) string {
	return idempotency.Fingerprint(
		strconv.FormatInt(taskID, 10),
		strconv.FormatInt(locationID, 10),
		strconv.Itoa(qty),
	)
}

// reconcilePutaway 业务事务回滚后，用新读核对已提交的同 key 幂等记录（语义与拣货一致）：
// 命中且指纹一致且空成功标记合法 → 回放成功；指纹不同 → 409；记录损坏 → 内部错误；
// 未命中/读取失败/请求已取消 → handled=false，保留原业务错误。
func (s *Service) reconcilePutaway(ctx context.Context, tenantID int64, idempotencyKey, requestHash string) (error, bool) {
	if cancelErr := ctx.Err(); cancelErr != nil {
		// 请求已取消：不发起核对查询；handled=false，调用方保留原业务错误。
		return cancelErr, false
	}
	record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, putawayIdempotencyScope, idempotencyKey)
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
