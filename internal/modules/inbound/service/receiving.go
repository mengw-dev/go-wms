package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tx"
)

// 收货业务。

func (s *Service) Receive(ctx context.Context, orderID, detailID int64, req *dto.ReceiveReq, operator string) error {
	return s.tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			return errcode.OrderNotFound
		}
		if o.Status != model.OrderApproved && o.Status != model.OrderReceiving {
			return errcode.OrderStatusWrong
		}
		d, err := s.repo.GetDetailForUpdate(tx, detailID)
		if err != nil {
			return errcode.OrderNotFound
		}
		if d.OrderID != orderID {
			return errcode.ParamError
		}
		remaining := d.ExpectedQty - d.ReceivedQty - d.DefectiveQty
		if req.Qty+req.DefectiveQty > remaining {
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
		// 收货任务推进：完成量 = 良品 + 残品（残品同样经过收货作业）。
		// 收齐时任务完成量恰好等于目标量，任务自动 CREATED → IN_PROGRESS → COMPLETED。
		if err := s.taskAPI.AddProgressByDetail(ctx, tx, orderID, detailID, taskmodel.TaskReceive, req.Qty+req.DefectiveQty, operator); err != nil {
			return err
		}

		// 重读全部明细（同事务可见原子累加后的新值）判断是否全部收齐
		all, err := s.repo.ListDetails(tx, orderID)
		if err != nil {
			return err
		}
		fullyReceived := true
		for _, item := range all {
			if item.ReceivedQty+item.DefectiveQty < item.ExpectedQty {
				fullyReceived = false
				break
			}
		}
		var toStatus model.OrderStatus
		var putawayTasks []*taskapi.CreateTask
		if fullyReceived {
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
		} else if o.Status == model.OrderApproved {
			// 状态机校验：首次部分收货 APPROVED → RECEIVING
			if !model.CanTransit(o.Status, model.OrderReceiving) {
				return errcode.OrderStatusWrong
			}
			toStatus = model.OrderReceiving
		} else {
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
		return nil
	})
}
