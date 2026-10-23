package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/quantity"
	"gowms/internal/pkg/snowflake"
)

func closureReason(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 255 {
		return "", errcode.ParamError
	}
	return value, nil
}
func closureHash(action, operator string, payload any) idempotency.RequestFingerprint {
	raw, _ := json.Marshal(payload)
	return idempotency.Fingerprint(action, operator, string(raw))
}

func (s *Service) ListExceptions(ctx context.Context, q *dto.ClosureQuery) ([]*model.WorkException, int64, error) {
	return s.repo.ListExceptions(s.tm.DB().WithContext(waveContext(ctx)), q.OrderID, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ReportException(ctx context.Context, req *dto.ExceptionRequest, operator, key string) (*dto.ClosureAck, error) {
	if req == nil || !req.ConfirmReturned || req.TaskID <= 0 || req.Quantity <= 0 || req.Quantity > quantity.Max || (req.Kind != "SHORTAGE" && req.Kind != "DAMAGE") || operator == "" {
		return nil, errcode.ParamError
	}
	reason, e := closureReason(req.Reason)
	if e != nil {
		return nil, e
	}
	input := *req
	input.Reason = reason
	ctx = waveContext(ctx)
	routing, e := s.taskAPI.Get(ctx, input.TaskID)
	if e != nil {
		return nil, e
	}
	id, e := idempotency.Command(ctx, s.tm, "outbound.exception.report", key, closureHash("report", operator, input), func(db *gorm.DB) (int64, error) {
		// This routing read is outside the business snapshot. Authoritative rows are
		// locked tasks -> allocations -> inventory -> order, as ordinary picking.
		tasks, e := s.taskAPI.ListByOrderForUpdate(ctx, db, routing.OrderID)
		if e != nil {
			return 0, e
		}
		var selected *taskmodel.Task
		for _, t := range tasks {
			if t.WaveID != 0 {
				return 0, errcode.WaveOrderLocked
			}
			if t.ID == input.TaskID {
				selected = t
			}
		}
		if selected == nil || selected.TaskType != taskmodel.TaskPick || (selected.Status != taskmodel.TaskCreated && selected.Status != taskmodel.TaskInProgress) || input.Quantity > selected.TargetQty-selected.DoneQty {
			return 0, errcode.ExceptionNotAllowed
		}
		rows, e := s.repo.ListAllocations(db, selected.OrderID)
		if e != nil {
			return 0, e
		}
		allocations := make([]*model.Allocation, 0, len(rows))
		var source *model.Allocation
		for _, a := range rows {
			locked, e := s.repo.GetAllocationForUpdate(db, a.ID)
			if e != nil {
				return 0, e
			}
			allocations = append(allocations, locked)
			if a.ID == selected.AllocationID {
				source = locked
			}
		}
		if source == nil || source.Status != model.AllocAllocated || source.PickedQty != selected.DoneQty || source.AllocatedQty != selected.TargetQty || source.SKUID != selected.SKUID || source.OrderID != selected.OrderID || source.DetailID != selected.DetailID || source.LocationID != selected.LocationID || source.BatchNo != selected.BatchNo {
			return 0, errcode.ExceptionNotAllowed
		}
		sort.Slice(allocations, func(i, j int) bool { return allocations[i].InventoryID < allocations[j].InventoryID })
		remaining, shipped := int64(0), 0
		for _, a := range allocations {
			if a.Status == model.AllocPicked {
				shipped += a.PickedQty
			}
			if a.Status != model.AllocAllocated {
				continue
			}
			q := a.AllocatedQty
			if q < 0 {
				return 0, errcode.ExceptionNotAllowed
			}
			if q == 0 {
				continue
			}
			remaining += int64(q)
			if e = s.inv.Release(ctx, db, &invapi.ReleaseReq{InventoryID: a.InventoryID, Quantity: q, OrderNo: selected.OrderNo, Operator: operator}); e != nil {
				return 0, e
			}
		}
		stockReq := &invapi.FreezeReq{InventoryID: source.InventoryID, Quantity: input.Quantity, Reason: reason, OrderNo: selected.OrderNo, Operator: operator}
		if input.Kind == "DAMAGE" {
			e = s.inv.Freeze(ctx, db, stockReq)
		} else {
			e = s.inv.WriteOff(ctx, db, stockReq)
		}
		if e != nil {
			return 0, e
		}
		order, e := s.repo.GetOrderForUpdate(db, selected.OrderID)
		if e != nil {
			return 0, e
		}
		if order.Status != model.OrderPicking || remaining != int64(order.AllocatedQty-shipped) || remaining == 0 {
			return 0, errcode.ExceptionNotAllowed
		}
		if e = s.taskAPI.CancelByOrder(ctx, db, order.ID); e != nil {
			return 0, e
		}
		order.PickedQty = shipped
		if e = s.repo.StopRemaining(db, order, allocations); e != nil {
			return 0, e
		}
		row := &model.WorkException{ID: snowflake.Next(), OrderID: order.ID, OrderNo: order.OrderNo, TaskID: selected.ID, InventoryID: source.InventoryID, Kind: input.Kind, Quantity: input.Quantity, Reason: reason, Status: "OPEN", Operator: operator}
		return row.ID, s.repo.SaveException(db, row)
	})
	return &dto.ClosureAck{ObjectID: id}, e
}

func (s *Service) ResolveWorkException(ctx context.Context, id int64, req *dto.ResolveExceptionRequest, operator, key string) (*dto.ClosureAck, error) {
	if req == nil || id <= 0 || operator == "" {
		return nil, errcode.ParamError
	}
	reason, e := closureReason(req.Reason)
	if e != nil {
		return nil, e
	}
	input := struct {
		ID     int64
		Close  bool
		Reason string
	}{id, req.Close, reason}
	ctx = waveContext(ctx)
	out, e := idempotency.Command(ctx, s.tm, "outbound.exception.resolve", key, closureHash("resolve", operator, input), func(db *gorm.DB) (int64, error) {
		row, e := s.repo.Exception(db, id)
		if e != nil {
			return 0, e
		}
		if row.Status != "OPEN" {
			return 0, errcode.ExceptionNotAllowed
		}
		order, e := s.repo.GetOrderForUpdate(db, row.OrderID)
		if e != nil {
			return 0, e
		}
		if order.Status != model.OrderException || order.AllocatedQty != order.PickedQty {
			return 0, errcode.ExceptionNotAllowed
		}
		if req.Close {
			order.Status = model.OrderClosed
		} else {
			details, e := s.repo.ListDetailsForApproval(db, order.ID, s.limits.MaxShipmentDetails)
			if e != nil {
				return 0, e
			}
			if len(details) > s.limits.MaxShipmentDetails {
				return 0, errcode.ShipDetailScaleExceeded
			}
			sort.Slice(details, func(i, j int) bool { return details[i].SKUID < details[j].SKUID })
			allocations := make([]*model.Allocation, 0)
			for _, d := range details {
				need := d.ExpectedQty - d.PickedQty
				if need < 0 {
					return 0, errcode.ExceptionNotAllowed
				}
				if need == 0 {
					continue
				}
				if len(allocations) >= s.limits.MaxAllocationRows {
					return 0, errcode.AllocationScaleExceeded
				}
				result, e := s.inv.Allocate(ctx, db, &invapi.AllocateReq{WarehouseID: order.WarehouseID, SKUID: d.SKUID, Quantity: need, OrderNo: order.OrderNo, Operator: operator, MaxRows: s.limits.MaxAllocationRows - len(allocations)})
				if e != nil {
					return 0, e
				}
				for _, a := range result.Rows {
					allocations = append(allocations, &model.Allocation{Base: modelbase.Base{ID: snowflake.Next()}, OrderID: order.ID, DetailID: d.ID, InventoryID: a.InventoryID, SKUID: d.SKUID, LocationID: a.LocationID, LocationCode: a.LocationCode, BatchNo: a.BatchNo, AllocatedQty: a.Quantity, Status: model.AllocAllocated})
				}
				d.AllocatedQty = d.ExpectedQty
				if e = s.repo.UpdateDetailAllocated(db, d); e != nil {
					return 0, e
				}
			}
			if len(allocations) == 0 {
				return 0, errcode.ExceptionNotAllowed
			}
			if e = s.repo.CreateAllocations(db, allocations); e != nil {
				return 0, e
			}
			tasks := make([]*taskapi.CreateTask, 0, len(allocations))
			for _, a := range allocations {
				tasks = append(tasks, &taskapi.CreateTask{TaskType: taskmodel.TaskPick, OrderID: order.ID, OrderNo: order.OrderNo, DetailID: a.DetailID, AllocationID: a.ID, SKUID: a.SKUID, WarehouseID: order.WarehouseID, LocationID: a.LocationID, LocationCode: a.LocationCode, BatchNo: a.BatchNo, TargetQty: a.AllocatedQty})
			}
			if e = s.taskAPI.Create(ctx, db, tasks); e != nil {
				return 0, e
			}
			order.AllocatedQty = order.ExpectedQty
			order.Status = model.OrderPicking
		}
		n, e := s.repo.UpdateOrderProgress(db, order)
		if e != nil {
			return 0, e
		}
		if n != 1 {
			return 0, errcode.Conflict
		}
		return id, s.repo.ResolveException(db, id, req.Close, reason, operator)
	})
	return &dto.ClosureAck{ObjectID: out}, e
}
