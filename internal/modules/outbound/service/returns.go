package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/quantity"
	"gowms/internal/pkg/snowflake"
)

func (s *Service) ListReturns(ctx context.Context, q *dto.ClosureQuery) ([]*model.CustomerReturn, int64, error) {
	return s.repo.ListReturns(s.tm.DB().WithContext(waveContext(ctx)), q.OrderID, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ReturnLocations(ctx context.Context, id int64, q *dto.ReturnLocationQuery) ([]*dto.ReturnLocation, int64, error) {
	ctx = waveContext(ctx)
	db := s.tm.DB().WithContext(ctx)
	order, e := s.repo.GetOrder(ctx, db, id)
	if e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, 0, errcode.ShipOrderNotFound
		}
		return nil, 0, e
	}
	if order.Status != model.OrderShipped && order.Status != model.OrderClosed {
		return nil, 0, errcode.ReturnNotAllowed
	}
	return s.repo.ReturnLocations(db, order.WarehouseID, q)
}
func (s *Service) Returnable(ctx context.Context, id int64) ([]*dto.ReturnableRow, error) {
	if id <= 0 {
		return nil, errcode.ParamError
	}
	return s.repo.Returnable(s.tm.DB().WithContext(waveContext(ctx)), id)
}

func (s *Service) ReceiveCustomerReturn(ctx context.Context, req *dto.CustomerReturnRequest, operator, key string) (*dto.ClosureAck, error) {
	if req == nil || req.AllocationID <= 0 || req.LocationID <= 0 || req.GoodQty < 0 || req.BadQty < 0 || req.GoodQty > quantity.Max-req.BadQty || req.GoodQty+req.BadQty <= 0 || operator == "" {
		return nil, errcode.ParamError
	}
	reason, e := closureReason(req.Reason)
	if e != nil {
		return nil, e
	}
	input := *req
	input.Reason = reason
	ctx = waveContext(ctx)
	id, e := idempotency.Command(ctx, s.tm, "outbound.customer-return", key, closureHash("receive-return", operator, input), func(db *gorm.DB) (int64, error) {
		// The first read is a lock; the aggregate below must see the previous return
		// after waiting for this allocation, not an earlier RR snapshot.
		allocation, e := s.repo.GetAllocationForUpdate(db, input.AllocationID)
		if e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return 0, errcode.ReturnNotAllowed
			}
			return 0, e
		}
		order, e := s.repo.GetOrderForUpdate(db, allocation.OrderID)
		if e != nil {
			return 0, e
		}
		if (order.Status != model.OrderShipped && order.Status != model.OrderClosed) || allocation.Status != model.AllocPicked || allocation.PickedQty != allocation.AllocatedQty || allocation.PickedQty <= 0 {
			return 0, errcode.ReturnNotAllowed
		}
		detail, e := s.repo.GetDetailForUpdate(db, allocation.DetailID)
		if e != nil {
			return 0, e
		}
		if detail.OrderID != order.ID || detail.SKUID != allocation.SKUID || detail.PickedQty < allocation.PickedQty || order.PickedQty < allocation.PickedQty {
			return 0, errcode.ReturnNotAllowed
		}
		returned, e := s.repo.ReturnedQty(db, allocation.ID)
		if e != nil {
			return 0, e
		}
		if returned+int64(input.GoodQty)+int64(input.BadQty) > int64(allocation.PickedQty) {
			return 0, errcode.ReturnNotAllowed
		}
		if e = s.basic.ValidateLocationInWarehouseInTx(ctx, db, order.WarehouseID, input.LocationID); e != nil {
			return 0, e
		}
		if e = s.basic.ValidateWarehouseInTx(ctx, db, order.WarehouseID); e != nil {
			return 0, e
		}
		location, e := s.basic.GetLocationInTx(ctx, db, input.LocationID)
		if e != nil {
			return 0, e
		}
		skus, e := s.basic.GetSKUsByIDsForShare(ctx, db, []int64{allocation.SKUID})
		if e != nil {
			return 0, e
		}
		if skus[allocation.SKUID].Status != 1 {
			return 0, errcode.SKUDisabled
		}
		id := snowflake.Next()
		no := fmt.Sprintf("TH%s%d", time.Now().Format("20060102"), id)
		if e = s.inv.ReceiveReturn(ctx, db, &invapi.ReturnReq{WarehouseID: order.WarehouseID, LocationID: input.LocationID, SKUID: allocation.SKUID, BatchNo: allocation.BatchNo, GoodQty: input.GoodQty, BadQty: input.BadQty, OrderNo: no, Operator: operator, Reason: reason}); e != nil {
			return 0, e
		}
		if e = s.basic.UpdateLocationStatusInTx(ctx, db, input.LocationID, 2); e != nil {
			return 0, e
		}
		return id, s.repo.SaveReturn(db, &model.CustomerReturn{ID: id, ReturnNo: no, OrderID: order.ID, OrderNo: order.OrderNo, AllocationID: allocation.ID, WarehouseID: order.WarehouseID, LocationID: input.LocationID, LocationCode: location.Code, SKUID: allocation.SKUID, SKUCode: skus[allocation.SKUID].Code, SKUName: skus[allocation.SKUID].Name, Unit: skus[allocation.SKUID].Unit, BatchNo: allocation.BatchNo, GoodQty: input.GoodQty, BadQty: input.BadQty, Reason: reason, Operator: operator})
	})
	return &dto.ClosureAck{ObjectID: id}, e
}
