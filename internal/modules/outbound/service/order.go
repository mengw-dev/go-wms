package service

import (
	"context"
	"sort"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	sysmodel "gowms/internal/modules/system/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
)

// 出库单据生命周期与明细构建。

func (s *Service) Create(ctx context.Context, req *dto.CreateOrderReq, operator string) (*model.ShipmentOrder, error) {
	if err := s.basic.ValidateWarehouse(ctx, req.WarehouseID); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetOrderByBizNo(ctx, s.tm.DB(), req.BizOrderNo); err == nil {
		return nil, errcode.BizOrderDuplicate
	}
	details, expected, err := s.buildDetails(ctx, req.Details)
	if err != nil {
		return nil, err
	}
	var order *model.ShipmentOrder
	for i := 0; i < tx.MaxOrderNoRetry; i++ {
		order = &model.ShipmentOrder{
			Base: sysmodel.Base{ID: snowflake.Next()}, OrderNo: s.no.Next(ctx, model.OrderNoPrefix),
			BizOrderNo: req.BizOrderNo, WarehouseID: req.WarehouseID,
			Status: model.OrderDraft, Remark: req.Remark, ExpectedQty: expected, CreatedBy: operator,
		}
		err = s.tm.Tx(ctx, func(tx *gorm.DB) error {
			return s.repo.CreateOrder(tx, order, details)
		})
		if err == nil {
			return order, nil
		}
		if !tx.IsDuplicateErr(err) {
			return nil, err
		}
		log.WithContext(ctx).Warn("order_no duplicated, retry", "order_no", order.OrderNo)
	}
	return nil, errcode.OrderNoDuplicate
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.ShipOrderNotFound
		}
		if o.Status != model.OrderDraft {
			return errcode.ShipOrderStatusWrong
		}
		return s.repo.DeleteOrder(tx, id)
	})
}

func (s *Service) Submit(ctx context.Context, id int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.ShipOrderNotFound
		}
		if !model.CanTransit(o.Status, model.OrderSubmitted) {
			return errcode.ShipOrderStatusWrong
		}
		if n, err := s.repo.UpdateStatus(tx, id, o.Status, model.OrderSubmitted); err != nil {
			return err
		} else if n == 0 {
			return errcode.ShipOrderVersionBad
		}
		return nil
	})
}

func (s *Service) Approve(ctx context.Context, id int64, operator string) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.ShipOrderNotFound
		}
		if o.Status != model.OrderSubmitted {
			return errcode.ShipOrderStatusWrong
		}
		details, err := s.repo.ListDetails(tx, id)
		if err != nil {
			return err
		}
		// 按 SKU_ID 排序后再分配：所有事务库存行加锁顺序全局一致，
		// 避免并发审核交叉加锁（T1 持 A 等 B，T2 持 B 等 A）导致死锁
		sort.Slice(details, func(i, j int) bool { return details[i].SKUID < details[j].SKUID })

		// 逐明细 FIFO 分配（失败即整体回滚）
		allocations := make([]*model.Allocation, 0, len(details))
		for _, d := range details {
			result, err := s.inv.Allocate(ctx, tx, &invapi.AllocateReq{
				WarehouseID: o.WarehouseID, SKUID: d.SKUID,
				Quantity: d.ExpectedQty, OrderNo: o.OrderNo, Operator: operator,
			})
			if err != nil {
				return err // 可用不足：明确报错"需要N，实际可用M"，事务回滚
			}
			d.AllocatedQty = d.ExpectedQty
			if err := s.repo.UpdateDetailAllocated(tx, d); err != nil {
				return err
			}
			for _, row := range result.Rows {
				allocations = append(allocations, &model.Allocation{
					Base: sysmodel.Base{ID: snowflake.Next()}, OrderID: o.ID, DetailID: d.ID,
					InventoryID: row.InventoryID, SKUID: d.SKUID,
					LocationID: row.LocationID, LocationCode: row.LocationCode, BatchNo: row.BatchNo,
					AllocatedQty: row.Quantity, Status: model.AllocAllocated,
				})
			}
		}
		// 按储位编码排序：FIFO 分配结果不变（扣哪个批次多少个已定），
		// 只调整拣货顺序，让拣货员按储位字典序走一遍，优化拣货路径。
		sort.Slice(allocations, func(i, j int) bool {
			return allocations[i].LocationCode < allocations[j].LocationCode
		})
		if err := s.repo.CreateAllocations(tx, allocations); err != nil {
			return err
		}

		// 状态机校验 + 主单累加分配量，状态推进 SUBMITTED → PICKING（审核即分配）
		if !model.CanTransit(o.Status, model.OrderPicking) {
			return errcode.ShipOrderStatusWrong
		}
		o.AllocatedQty = o.ExpectedQty
		o.Status = model.OrderPicking
		if n, err := s.repo.UpdateOrderProgress(tx, o); err != nil {
			return err
		} else if n == 0 {
			return errcode.ShipOrderVersionBad
		}

		// 按分配行生成拣货任务
		tasks := make([]*taskapi.CreateTask, 0, len(allocations))
		for _, a := range allocations {
			tasks = append(tasks, &taskapi.CreateTask{
				TaskType: taskmodel.TaskPick, OrderID: o.ID, OrderNo: o.OrderNo,
				AllocationID: a.ID, SKUID: a.SKUID, WarehouseID: o.WarehouseID, TargetQty: a.AllocatedQty,
				LocationID: a.LocationID, LocationCode: a.LocationCode, BatchNo: a.BatchNo,
			})
		}
		return s.taskAPI.Create(ctx, tx, tasks)
	})
}

func (s *Service) Cancel(ctx context.Context, id int64, operator string) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			return errcode.ShipOrderNotFound
		}
		// 状态机校验：终态 SHIPPED/CANCELLED 不在转换表中，直接拒绝取消
		if !model.CanTransit(o.Status, model.OrderCancelled) {
			return errcode.ShipOrderStatusWrong
		}
		switch o.Status {
		case model.OrderDraft, model.OrderSubmitted:
			if n, err := s.repo.UpdateStatus(tx, id, o.Status, model.OrderCancelled); err != nil {
				return err
			} else if n == 0 {
				return errcode.ShipOrderVersionBad
			}
			return s.taskAPI.CancelByOrder(ctx, tx, id)
		case model.OrderApproved, model.OrderPicking:
			if o.PickedQty > 0 {
				return errcode.ShipShippedForbidden
			}
		default:
			return errcode.ShipOrderStatusWrong
		}

		// 释放已锁定库存
		allocations, err := s.repo.ListAllocations(tx, id)
		if err != nil {
			return err
		}
		for _, a := range allocations {
			if a.Status != model.AllocAllocated {
				continue
			}
			if err := s.inv.Release(ctx, tx, &invapi.ReleaseReq{
				InventoryID: a.InventoryID, Quantity: a.AllocatedQty,
				OrderNo: o.OrderNo, Operator: operator,
			}); err != nil {
				return err
			}
		}
		if n, err := s.repo.CancelAllocationsByOrder(tx, id); err != nil {
			return err
		} else if n == 0 && len(allocations) > 0 {
			return errcode.AllocConflict
		}
		if n, err := s.repo.UpdateStatus(tx, id, o.Status, model.OrderCancelled); err != nil {
			return err
		} else if n == 0 {
			return errcode.ShipOrderVersionBad
		}
		return s.taskAPI.CancelByOrder(ctx, tx, id)
	})
}

// batchOper 逐张执行，部分成功不回滚，返回明细。
func (s *Service) batchOper(ctx context.Context, ids []int64, operator string, fn func(context.Context, int64, string) error) *dto.BatchOperResp {
	resp := &dto.BatchOperResp{}
	for _, id := range ids {
		if err := fn(ctx, id, operator); err != nil {
			resp.Fail++
			resp.Errors = append(resp.Errors, dto.BatchItemError{ID: id, Msg: err.Error()})
			continue
		}
		resp.Success++
	}
	return resp
}

// BatchDelete 批量删除（仅 DRAFT）。
func (s *Service) BatchDelete(ctx context.Context, ids []int64) *dto.BatchOperResp {
	resp := &dto.BatchOperResp{}
	for _, id := range ids {
		if err := s.Delete(ctx, id); err != nil {
			resp.Fail++
			resp.Errors = append(resp.Errors, dto.BatchItemError{ID: id, Msg: err.Error()})
			continue
		}
		resp.Success++
	}
	return resp
}

// BatchSubmit 批量提交（DRAFT → SUBMITTED）。
func (s *Service) BatchSubmit(ctx context.Context, ids []int64) *dto.BatchOperResp {
	resp := &dto.BatchOperResp{}
	for _, id := range ids {
		if err := s.Submit(ctx, id); err != nil {
			resp.Fail++
			resp.Errors = append(resp.Errors, dto.BatchItemError{ID: id, Msg: err.Error()})
			continue
		}
		resp.Success++
	}
	return resp
}

// BatchApprove 批量审核（SUBMITTED → PICKING，含库存分配 + 拣货任务生成）。
func (s *Service) BatchApprove(ctx context.Context, ids []int64, operator string) *dto.BatchOperResp {
	return s.batchOper(ctx, ids, operator, s.Approve)
}

// BatchCancel 批量作废（DRAFT/SUBMITTED/APPROVED/PICKING → CANCELLED，释放已分配库存）。
func (s *Service) BatchCancel(ctx context.Context, ids []int64, operator string) *dto.BatchOperResp {
	return s.batchOper(ctx, ids, operator, s.Cancel)
}

func (s *Service) buildDetails(ctx context.Context, items []dto.OrderDetailItem) ([]*model.ShipmentOrderDetail, int, error) {
	details := make([]*model.ShipmentOrderDetail, 0, len(items))
	expected := 0
	seen := map[int64]struct{}{}
	for _, it := range items {
		if _, dup := seen[it.SKUID]; dup {
			return nil, 0, errcode.ShipDetailDuplicateSKU
		}
		seen[it.SKUID] = struct{}{}
		sku, err := s.basic.GetSKU(ctx, it.SKUID)
		if err != nil {
			return nil, 0, err
		}
		details = append(details, &model.ShipmentOrderDetail{
			Base: sysmodel.Base{ID: snowflake.Next()}, SKUID: sku.ID, SKUCode: sku.Code, SKUName: sku.Name,
			ExpectedQty: it.ExpectedQty,
		})
		expected += it.ExpectedQty
	}
	return details, expected, nil
}
