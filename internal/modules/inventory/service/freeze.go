package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/quantity"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

// Freeze reclassifies available/frozen stock; it never touches another order's reservations.
func (s *Service) Freeze(ctx context.Context, db *gorm.DB, req *api.FreezeReq) error {
	if req == nil || req.Quantity <= 0 || req.Quantity > quantity.Max {
		return errcode.ParamError
	}
	inv, e := s.repo.GetForUpdate(db, req.InventoryID)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return errcode.InventoryNotFound
	}
	if e != nil {
		return e
	}
	n, e := s.repo.FreezeQty(db, inv.ID, req.Quantity, req.Unfreeze)
	if e != nil {
		return e
	}
	if n != 1 {
		return errcode.FrozenNotEnough
	}
	delta, kind := req.Quantity, model.TransFreeze
	if req.Unfreeze {
		delta, kind = -delta, model.TransUnfreeze
	}
	return s.repo.InsertTrans(db, &model.InventoryTrans{ID: snowflake.Next(), InventoryID: inv.ID, TransType: kind, BeforeQuantity: inv.StockQuantity, AfterQuantity: inv.StockQuantity, AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty - delta, FrozenBefore: inv.FrozenQty, FrozenAfter: inv.FrozenQty + delta, OrderNo: req.OrderNo, Operator: req.Operator, Reason: req.Reason})
}

func (s *Service) FreezeStock(ctx context.Context, req *dto.FreezeRequest, key, operator string) (int64, error) {
	if req == nil || req.InventoryID <= 0 || operator == "" {
		return 0, errcode.ParamError
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 255 {
		return 0, errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	hash := idempotency.Fingerprint(strconv.FormatInt(req.InventoryID, 10), strconv.Itoa(req.Quantity), strconv.FormatBool(req.Unfreeze), reason, operator)
	return idempotency.Command(ctx, s.tm, "inventory.freeze", key, hash, func(db *gorm.DB) (int64, error) {
		return req.InventoryID, s.Freeze(ctx, db, &api.FreezeReq{InventoryID: req.InventoryID, Quantity: req.Quantity, Unfreeze: req.Unfreeze, Reason: reason, Operator: operator})
	})
}

// WriteOff removes confirmed missing physical stock, after its order reservation
// was released in this same transaction. Frozen/other allocated stock stays intact.
func (s *Service) WriteOff(ctx context.Context, db *gorm.DB, req *api.FreezeReq) error {
	if req == nil || req.Quantity <= 0 || req.Quantity > quantity.Max {
		return errcode.ParamError
	}
	inv, e := s.repo.GetForUpdate(db, req.InventoryID)
	if e != nil {
		return e
	}
	n, e := s.repo.AdjustNegative(db, inv.ID, req.Quantity)
	if e != nil {
		return e
	}
	if n != 1 {
		return errcode.FrozenNotEnough
	}
	return s.repo.InsertTrans(db, &model.InventoryTrans{ID: snowflake.Next(), InventoryID: inv.ID, TransType: model.TransShortage, QuantityChange: -req.Quantity, BeforeQuantity: inv.StockQuantity, AfterQuantity: inv.StockQuantity - req.Quantity, AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty - req.Quantity, FrozenBefore: inv.FrozenQty, FrozenAfter: inv.FrozenQty, Reason: req.Reason, OrderNo: req.OrderNo, Operator: req.Operator})
}

// ReceiveReturn uses the caller's transaction: receipt, good stock and bad stock
// are committed together; no bad quantity can be allocated between these writes.
func (s *Service) ReceiveReturn(ctx context.Context, db *gorm.DB, req *api.ReturnReq) error {
	if req == nil || req.GoodQty < 0 || req.BadQty < 0 || req.GoodQty > quantity.Max-req.BadQty || req.GoodQty+req.BadQty <= 0 {
		return errcode.ParamError
	}
	if e := s.Increase(ctx, db, &api.IncreaseReq{WarehouseID: req.WarehouseID, LocationID: req.LocationID, SKUID: req.SKUID, BatchNo: req.BatchNo, Quantity: req.GoodQty + req.BadQty, OrderNo: req.OrderNo, Operator: req.Operator}); e != nil {
		return e
	}
	if req.BadQty == 0 {
		return nil
	}
	inv, e := s.repo.GetByTupleForUpdate(db, req.WarehouseID, req.LocationID, req.SKUID, req.BatchNo)
	if e != nil {
		return e
	}
	return s.Freeze(ctx, db, &api.FreezeReq{InventoryID: inv.ID, Quantity: req.BadQty, Reason: req.Reason, OrderNo: req.OrderNo, Operator: req.Operator})
}
