package service

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tx"
)

// 拣货和发货扣减。

// PickScan 拣货前的扫码核对信息，为空字段表示该维度不校验。
type PickScan struct {
	LocationCode string
	BatchNo      string
}

// Pick 按分配行拣货。scan 非空时校验扫描的库位/批次与任务一致，
// 避免同一库位下不同批次被拣错。
func (s *Service) Pick(ctx context.Context, taskID int64, qty int, operator string, scan *PickScan) error {
	// 事务外只读不可变路由信息（OrderID/AllocationID/TaskType/TaskNo 建后不变）
	routing, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if routing.TaskType != taskmodel.TaskPick {
		return errcode.TaskStatusWrong
	}
	return s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		// 先锁主单，与 Cancel 串行处理同一张单据，再锁任务和分配行。
		o, err := s.repo.GetOrderForUpdate(tx, routing.OrderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.ShipOrderNotFound
			}
			return err
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
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.ShipOrderNotFound
			}
			return err
		}
		if a.Status != model.AllocAllocated {
			return errcode.TaskStatusWrong
		}
		if err := checkPickScan(scan, t); err != nil {
			return err
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

// checkPickScan 核对拣货员扫描的库位和批次是否与任务要求一致（忽略大小写和首尾空格）。
func checkPickScan(scan *PickScan, t *taskmodel.Task) error {
	if scan == nil {
		return nil
	}
	if code := strings.TrimSpace(scan.LocationCode); code != "" && !strings.EqualFold(code, t.LocationCode) {
		return errcode.PickLocationMismatch
	}
	if batch := strings.TrimSpace(scan.BatchNo); batch != "" && !strings.EqualFold(batch, t.BatchNo) {
		return errcode.PickBatchMismatch
	}
	return nil
}
