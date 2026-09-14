package service

import (
	"context"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tx"
)

// 拣货和发货扣减。

func (s *Service) Pick(ctx context.Context, taskID int64, qty int, operator string) error {
	// 事务外只读不可变路由信息（OrderID/AllocationID/TaskType/TaskNo 建后不变）
	routing, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return errcode.TaskNotFound
	}
	if routing.TaskType != taskmodel.TaskPick {
		return errcode.TaskStatusWrong
	}
	return s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		// 锁顺序：主单 → 任务 → 分配行（与 Cancel 的 单据→分配行→任务 保持一致，降低死锁概率）
		o, err := s.repo.GetOrderForUpdate(tx, routing.OrderID)
		if err != nil {
			return errcode.ShipOrderNotFound
		}
		if o.Status != model.OrderPicking {
			return errcode.ShipOrderStatusWrong
		}
		// 事务内行锁读取任务：权威校验任务未被并发取消（AddProgress 内部也会锁读复核）
		t, err := s.taskAPI.GetForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if t.Status != taskmodel.TaskCreated && t.Status != taskmodel.TaskInProgress {
			return errcode.TaskStatusWrong
		}
		a, err := s.repo.GetAllocationForUpdate(tx, routing.AllocationID)
		if err != nil {
			return errcode.ShipOrderNotFound
		}
		if a.Status != model.AllocAllocated {
			return errcode.TaskStatusWrong
		}
		// 推进拣货任务（内部行锁 + 校验数量不超剩余 + 任务状态机）
		if err := s.taskAPI.AddProgress(ctx, tx, taskID, qty, operator); err != nil {
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
		// 主单原子累加拣货量（不依赖内存对象回写）
		if n, err := s.repo.IncrOrderPicked(tx, o.ID, o.Version, qty); err != nil {
			return err
		} else if n == 0 {
			return errcode.ShipOrderVersionBad
		}
		// 明细原子累加
		if err := s.repo.IncrDetailPicked(tx, a.DetailID, qty); err != nil {
			return err
		}

		// 分配行拣满 → 发货扣减库存
		if allocFullyPicked {
			if err := s.inv.Ship(ctx, tx, &invapi.ShipReq{
				InventoryID: a.InventoryID, Quantity: a.AllocatedQty,
				OrderNo: o.OrderNo, TaskNo: t.TaskNo, Operator: operator,
			}); err != nil {
				return err
			}
		}
		// 主单全部拣完 → SHIPPED（行锁内 base+delta 决策，状态机校验 + CAS 兜底）
		newOrderPicked := o.PickedQty + qty
		if o.AllocatedQty-newOrderPicked == 0 {
			if !model.CanTransit(o.Status, model.OrderShipped) {
				return errcode.ShipOrderStatusWrong
			}
			if n, err := s.repo.UpdateStatus(tx, o.ID, model.OrderPicking, model.OrderShipped); err != nil {
				return err
			} else if n == 0 {
				return errcode.ShipOrderVersionBad
			}
		}
		return nil
	})
}
