package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inbound/model"
	invapi "gowms/internal/modules/inventory/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tx"
)

// 上架业务。

func (s *Service) Putaway(ctx context.Context, taskID, locationID int64, qty int, operator string) error {
	// 事务外只读不可变路由信息（OrderID/DetailID/SKUID/TaskType/TaskNo/仓库 建后不变）
	routing, err := s.taskAPI.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if routing.TaskType != taskmodel.TaskPutaway {
		return errcode.TaskStatusWrong
	}
	// 库位校验：存在、非禁用，且必须属于单据仓库，防止跨仓库上架产生不一致库存
	if err := s.basic.ValidateLocationInWarehouse(ctx, routing.WarehouseID, locationID); err != nil {
		return err
	}
	return s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
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
		// 推进任务（含状态机与数量校验）
		if err := s.taskAPI.AddProgress(ctx, tx, taskID, qty, operator); err != nil {
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
		return nil
	})
}
