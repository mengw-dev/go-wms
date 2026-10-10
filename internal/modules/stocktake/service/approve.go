package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

// 盘点审核和库存调整。

// approveIdempotencyScope 盘点审核命令的幂等作用域（与收货/上架/拣货区分）。
const approveIdempotencyScope = "stocktake.approve"

// Approve 盘点审核与库存调整。终态防重复调整由状态机与明细 adjusted 标记保证；
// idempotencyKey 非空时补请求重放语义：同 key 重试回放空成功（data:null 契约不变），
// 审核成功但响应丢失后的重试不会因「已终态」被误报为业务失败。
// 同 key 被用于其他单据返回 409；未携带 key 保持旧契约但无请求级去重保证。
func (s *Service) Approve(ctx context.Context, orderID int64, operator, idempotencyKey string) error {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > 64 {
		return errcode.ParamError
	}
	tenantID := tenant.FromContext(ctx)
	var requestHash string
	if idempotencyKey != "" {
		// 审核接口只接受单据 ID（无明细请求体），指纹即单据：不读库拼可变明细。
		requestHash = idempotency.Fingerprint(strconv.FormatInt(orderID, 10))
	}
	// 幂等预查询放在事务外（独立连接，查完即还）：
	// 放在事务内会占住事务连接、再向同一连接池申请第二条连接，并发事务占满池时
	// 所有请求互相等待直至取消/超时；且事务内第一条普通 SELECT 会提前固定
	// REPEATABLE READ 读视图，交错录入实盘时审核会按过期数量调整库存。
	// 并发同 key 撞唯一键仍由 TxRetry 重跑 + reconcileApprove 新读核对回放，
	// 业务写入与幂等 Insert 保持同一事务提交。
	if idempotencyKey != "" {
		record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, approveIdempotencyScope, idempotencyKey)
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
	err := s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.StocktakeNotFound
			}
			return err
		}
		if !model.CanTransit(o.Status, model.OrderCompleted) {
			return errcode.StocktakeStatusWrong
		}
		details, err := s.repo.ListDetails(tx, orderID)
		if err != nil {
			return err
		}
		// 审核前置校验：所有明细都必须录入实盘数量，任一条缺失即整单拒绝。
		// 校验通过前不调整库存、不写流水、不推进单据状态，避免未盘完的盘点单被审核。
		for _, d := range details {
			if d.ActualQty == nil {
				return errcode.StocktakeNotFullyCounted
			}
		}
		// 盘点之间按库存 ID 统一加锁顺序；与其他业务仍可能竞争，死锁由整事务重试处理。
		sort.Slice(details, func(i, j int) bool { return details[i].InventoryID < details[j].InventoryID })
		anyCounted := false
		for _, d := range details {
			if d.Adjusted {
				continue
			}
			anyCounted = true
			// 差异和流水使用同一份加锁后的库存；零差异也校验库存是否存在。
			diff, err := s.inv.Adjust(ctx, tx, &api.AdjustReq{
				InventoryID: d.InventoryID, NewStock: *d.ActualQty,
				OrderNo: o.OrderNo, Operator: operator,
			})
			if err != nil {
				return err
			}
			if err := s.repo.MarkAdjusted(tx, d.ID, diff); err != nil {
				return err
			}
		}
		if !anyCounted {
			return errcode.StocktakeNoDetail
		}
		if n, err := s.repo.UpdateStatus(tx, orderID, o.Status, model.OrderCompleted); err != nil {
			return err
		} else if n == 0 {
			return errcode.StocktakeVersionBad
		}
		// 幂等记录与调整流水、明细标记、状态推进同事务提交：失败随事务回滚，key 可复用。
		if idempotencyKey != "" {
			record := &idempotency.Record{
				ID: snowflake.Next(), TenantID: tenantID,
				Scope: approveIdempotencyScope, IdempotencyKey: idempotencyKey,
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
		// 已提交的记录，拿锁后又因终态被拒：改用事务外新读核对，不把后到者误报为新操作失败。
		if idempotencyKey != "" {
			if reconcileErr, handled := s.reconcileApprove(ctx, tenantID, idempotencyKey, requestHash); handled {
				return reconcileErr
			}
		}
		return err
	}
	return nil
}

// reconcileApprove 业务事务回滚后，用新读核对已提交的同 key 幂等记录（语义与拣货一致）：
// 命中且指纹一致且空成功标记合法 → 回放成功；指纹不同 → 409；记录损坏 → 内部错误；
// 未命中/读取失败/请求已取消 → handled=false，保留原业务错误。
func (s *Service) reconcileApprove(ctx context.Context, tenantID int64, idempotencyKey, requestHash string) (error, bool) {
	if cancelErr := ctx.Err(); cancelErr != nil {
		// 请求已取消：不发起核对查询；handled=false，调用方保留原业务错误。
		return cancelErr, false
	}
	record, err := idempotency.Find(s.tm.DB().WithContext(ctx), tenantID, approveIdempotencyScope, idempotencyKey)
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
