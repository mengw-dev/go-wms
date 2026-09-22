package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	sysmodel "gowms/internal/modules/system/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/quota"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
)

// 入库单据生命周期与明细构建。

func (s *Service) Create(ctx context.Context, req *dto.CreateOrderReq, operator string) (*model.ReceiptOrder, error) {
	return s.createOrder(ctx, req, operator, nil)
}

// CreateResponse 返回创建入库单的 HTTP 响应结构。
func (s *Service) CreateResponse(ctx context.Context, req *dto.CreateOrderReq, operator string) (*dto.OrderResp, error) {
	order, err := s.Create(ctx, req, operator)
	if err != nil {
		return nil, err
	}
	return orderResponse(order), nil
}

func (s *Service) createImportOrder(ctx context.Context, task *model.ImportTask, rowNo int, req *dto.CreateOrderReq, operator string) (*model.ReceiptOrder, error) {
	// 幂等检查：该行已建单则直接复用（补偿重跑）
	if o, err := s.repo.GetByImportRow(ctx, s.tm.DB(), task.TaskID, rowNo); err == nil {
		return o, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	o, err := s.createOrder(ctx, req, operator, func(db *gorm.DB, order *model.ReceiptOrder) error {
		if err := s.repo.LockImportExecution(db, task); err != nil {
			return err
		}
		order.Source = model.SourceImport
		order.ImportTaskID = &task.TaskID
		order.ImportRow = rowNo
		return nil
	})
	if err != nil {
		// 幂等兜底：重跑/并发撞 uk_import_row 唯一键，回读已有单视为成功
		if exist, gerr := s.repo.GetByImportRow(ctx, s.tm.DB(), task.TaskID, rowNo); gerr == nil {
			return exist, nil
		}
		return nil, err
	}
	return o, nil
}

func (s *Service) createOrder(ctx context.Context, req *dto.CreateOrderReq, operator string, prepare func(*gorm.DB, *model.ReceiptOrder) error) (*model.ReceiptOrder, error) {
	// 公开租户配额：手动建单与 Excel 导入建单都走这里，统一拦住无限写入。
	if err := quota.Guard(ctx, s.tm.DB(), &model.ReceiptOrder{}, s.limits.MaxReceiptOrders, 1, "入库单"); err != nil {
		return nil, err
	}
	if err := s.basic.ValidateWarehouse(ctx, req.WarehouseID); err != nil {
		return nil, err
	}
	details, expected, err := s.buildDetails(ctx, req.Details)
	if err != nil {
		return nil, err
	}
	var order *model.ReceiptOrder
	for i := 0; i < tx.MaxOrderNoRetry; i++ { // 单号冲突重试
		order = &model.ReceiptOrder{
			Base:        sysmodel.Base{ID: snowflake.Next()},
			OrderNo:     s.no.Next(ctx, model.OrderNoPrefix),
			WarehouseID: req.WarehouseID, Status: model.OrderDraft,
			Source: model.SourceManual, Remark: req.Remark, ExpectedQty: expected, CreatedBy: operator,
		}
		err = s.tm.Tx(ctx, func(tx *gorm.DB) error {
			if prepare != nil {
				if err := prepare(tx, order); err != nil {
					return err
				}
			}
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

func (s *Service) Update(ctx context.Context, id int64, req *dto.CreateOrderReq) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		if o.Status != model.OrderDraft {
			return errcode.OrderStatusWrong
		}
		if err := s.basic.ValidateWarehouse(ctx, req.WarehouseID); err != nil {
			return err
		}
		details, expected, err := s.buildDetails(ctx, req.Details)
		if err != nil {
			return err
		}
		o.WarehouseID = req.WarehouseID
		o.Remark = req.Remark
		o.ExpectedQty = expected
		res := tx.Model(&model.ReceiptOrder{}).Where("id = ? AND version = ?", id, o.Version).Updates(map[string]any{
			"warehouse_id": o.WarehouseID, "remark": o.Remark, "expected_qty": o.ExpectedQty,
			"version": gorm.Expr("version + 1"),
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.OrderVersionBad
		}
		return s.repo.ReplaceDetails(tx, id, details)
	})
}

func (s *Service) Delete(ctx context.Context, id int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		if o.Status != model.OrderDraft {
			return errcode.OrderStatusWrong
		}
		return s.repo.DeleteOrder(tx, id)
	})
}

func (s *Service) Submit(ctx context.Context, id int64) error {
	return s.transit(ctx, id, model.OrderDraft, model.OrderSubmitted)
}

func (s *Service) Approve(ctx context.Context, id int64, _ string) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		if o.Status != model.OrderSubmitted {
			return errcode.OrderStatusWrong
		}
		// 状态机校验：SUBMITTED → APPROVED
		if !model.CanTransit(o.Status, model.OrderApproved) {
			return errcode.OrderStatusWrong
		}
		details, err := s.repo.ListDetails(tx, id)
		if err != nil {
			return err
		}
		if n, err := s.repo.UpdateStatus(tx, id, model.OrderSubmitted, model.OrderApproved); err != nil || n == 0 {
			if err != nil {
				return err
			}
			return errcode.OrderVersionBad
		}
		tasks := make([]*taskapi.CreateTask, 0, len(details))
		for _, d := range details {
			tasks = append(tasks, &taskapi.CreateTask{
				TaskType: taskmodel.TaskReceive, OrderID: o.ID, OrderNo: o.OrderNo,
				DetailID: d.ID, SKUID: d.SKUID, WarehouseID: o.WarehouseID, TargetQty: d.ExpectedQty,
			})
		}
		return s.taskAPI.Create(ctx, tx, tasks)
	})
}

func (s *Service) Cancel(ctx context.Context, id int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		// 状态机校验：DRAFT/SUBMITTED/APPROVED 可取消；RECEIVING 之后不可取消
		if !model.CanTransit(o.Status, model.OrderCancelled) {
			return errcode.OrderStatusWrong
		}
		if n, err := s.repo.UpdateStatus(tx, id, o.Status, model.OrderCancelled); err != nil || n == 0 {
			if err != nil {
				return err
			}
			return errcode.OrderVersionBad
		}
		return s.taskAPI.CancelByOrder(ctx, tx, id)
	})
}

// batchOper 逐张执行，部分成功不回滚，返回明细。
func (s *Service) batchOper(ctx context.Context, ids []int64, fn func(context.Context, int64) error) *dto.BatchOperResp {
	resp := &dto.BatchOperResp{}
	for _, id := range ids {
		if err := fn(ctx, id); err != nil {
			resp.Fail++
			if errcode.From(err).Code == errcode.Internal.Code {
				log.WithContext(ctx).Error("batch operation failed", "order_id", id, "err", err)
			}
			resp.Errors = append(resp.Errors, dto.BatchItemError{ID: id, Msg: errcode.From(err).Msg})
			continue
		}
		resp.Success++
	}
	return resp
}

// BatchDelete 批量删除（仅 DRAFT）。
func (s *Service) BatchDelete(ctx context.Context, ids []int64) *dto.BatchOperResp {
	return s.batchOper(ctx, ids, s.Delete)
}

// BatchSubmit 批量提交（DRAFT → SUBMITTED）。
func (s *Service) BatchSubmit(ctx context.Context, ids []int64) *dto.BatchOperResp {
	return s.batchOper(ctx, ids, func(ctx context.Context, id int64) error {
		return s.transit(ctx, id, model.OrderDraft, model.OrderSubmitted)
	})
}

// BatchApprove 批量审核（SUBMITTED → APPROVED，含收货任务生成）。
func (s *Service) BatchApprove(ctx context.Context, ids []int64, operator string) *dto.BatchOperResp {
	return s.batchOper(ctx, ids, func(ctx context.Context, id int64) error {
		return s.Approve(ctx, id, operator)
	})
}

// BatchCancel 批量作废（DRAFT/SUBMITTED/APPROVED → CANCELLED）。
func (s *Service) BatchCancel(ctx context.Context, ids []int64) *dto.BatchOperResp {
	return s.batchOper(ctx, ids, s.Cancel)
}

// DeleteByImportTask 按导入批次删除 DRAFT 入库单，返回成功/失败明细。
func (s *Service) DeleteByImportTask(ctx context.Context, taskID string) (*dto.BatchOperResp, error) {
	ids, err := s.repo.ListIDsByImportTask(ctx, s.tm.DB(), taskID)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return &dto.BatchOperResp{}, nil
	}
	return s.BatchDelete(ctx, ids), nil
}

func (s *Service) transit(ctx context.Context, id int64, from, to model.OrderStatus) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.OrderNotFound
			}
			return err
		}
		// 状态机校验：查转换表，非法流转（跨状态、终态再转、重复提交）一律拒绝
		if !model.CanTransit(o.Status, to) || o.Status != from {
			return errcode.OrderStatusWrong
		}
		if n, err := s.repo.UpdateStatus(tx, id, o.Status, to); err != nil {
			return err
		} else if n == 0 {
			// 行锁内仍 CAS 失败：并发状态变更，语义为版本冲突而非状态非法
			return errcode.OrderVersionBad
		}
		return nil
	})
}

func (s *Service) buildDetails(ctx context.Context, items []dto.OrderDetailItem) ([]*model.ReceiptOrderDetail, int, error) {
	details := make([]*model.ReceiptOrderDetail, 0, len(items))
	expected := 0
	seen := map[int64]struct{}{}
	for _, it := range items {
		if _, dup := seen[it.SKUID]; dup {
			return nil, 0, errcode.DetailDuplicateSKU
		}
		seen[it.SKUID] = struct{}{}
		sku, err := s.basic.GetSKU(ctx, it.SKUID)
		if err != nil {
			return nil, 0, err
		}
		details = append(details, &model.ReceiptOrderDetail{
			Base:  sysmodel.Base{ID: snowflake.Next()},
			SKUID: sku.ID, SKUCode: sku.Code, SKUName: sku.Name,
			ExpectedQty: it.ExpectedQty,
		})
		expected += it.ExpectedQty
	}
	return details, expected, nil
}
