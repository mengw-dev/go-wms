package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/quantity"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

func movementResponse(m *model.Movement) *dto.Movement {
	return &dto.Movement{ID: m.ID, MoveNo: m.MoveNo, WarehouseID: m.WarehouseID, WarehouseCode: m.WarehouseCode,
		SourceLocationID: m.SourceLocationID, TargetLocationID: m.TargetLocationID, SourceInventoryID: m.SourceInventoryID, TargetInventoryID: m.TargetInventoryID, SourceLocationCode: m.SourceLocationCode, TargetLocationCode: m.TargetLocationCode,
		SKUID: m.SKUID, SKUCode: m.SKUCode, SKUName: m.SKUName, Unit: m.Unit, BatchNo: m.BatchNo, Quantity: m.Quantity, StockInTime: m.StockInTime, Operator: m.Operator, Reason: m.Reason, CreatedAt: m.CreatedAt}
}

func replayMove(m *model.Movement, hash idempotency.RequestFingerprint) (*dto.Movement, error) {
	if !hash.Matches(m.RequestHash) {
		return nil, errcode.IdempotencyKeyReused
	}
	if m.ID <= 0 || m.Quantity <= 0 || m.SourceInventoryID <= 0 || m.TargetInventoryID <= 0 || m.MoveNo == "" {
		return nil, errcode.Internal
	}
	return movementResponse(m), nil
}

// MoveStock moves available stock once, retaining an immutable receipt with its
// operation key. Replay is checked before mutable inventory/reference validation.
func (s *Service) MoveStock(ctx context.Context, req *dto.MoveRequest, key string, actorID int64, operator string) (*dto.Movement, error) {
	key = strings.TrimSpace(key)
	if req == nil || req.SourceInventoryID <= 0 || req.TargetLocationID <= 0 || req.Quantity <= 0 || req.Quantity > quantity.Max || actorID <= 0 || operator == "" || key == "" || len(key) > 64 {
		return nil, errcode.ParamError
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 255 {
		return nil, errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	hash := idempotency.Fingerprint(strconv.FormatInt(req.SourceInventoryID, 10), strconv.FormatInt(req.TargetLocationID, 10), strconv.Itoa(req.Quantity), reason, strconv.FormatInt(actorID, 10))
	if old, err := s.repo.FindMovement(s.tm.DB().WithContext(ctx), key); err != nil {
		return nil, err
	} else if old != nil {
		return replayMove(old, hash)
	}
	routing, err := s.repo.MovementSource(s.tm.DB().WithContext(ctx), req.SourceInventoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errcode.InventoryNotFound
	}
	if err != nil {
		return nil, err
	}
	var result *dto.Movement
	err = s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(db *gorm.DB) error {
		w, locations, sku, err := s.repo.LockMoveReferences(db, routing, req.TargetLocationID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.MoveNotAllowed
		}
		if err != nil {
			return err
		}
		// First ordinary read is after the shared warehouse gate; later same-key
		// arrivals see its predecessor's committed receipt under repeatable read.
		if old, e := s.repo.FindMovement(db, key); e != nil {
			return e
		} else if old != nil {
			result, e = replayMove(old, hash)
			return e
		}
		source, err := s.repo.GetForUpdate(db, req.SourceInventoryID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.InventoryNotFound
		}
		if err != nil {
			return err
		}
		if source.WarehouseID != routing.WarehouseID || source.LocationID != routing.LocationID || source.SKUID != routing.SKUID || source.BatchNo != routing.BatchNo {
			return errcode.Conflict
		}
		if source.AvailableQty < req.Quantity {
			return errcode.MoveInsufficient
		}
		target, err := s.repo.GetByTupleForUpdate(db, source.WarehouseID, req.TargetLocationID, source.SKUID, source.BatchNo)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			target = &model.Inventory{Base: modelbase.Base{ID: snowflake.Next()}, WarehouseID: source.WarehouseID, LocationID: req.TargetLocationID, SKUID: source.SKUID, BatchNo: source.BatchNo, StockInTime: source.StockInTime}
			if err = s.repo.Create(db, target); err != nil {
				if pkgtx.IsDuplicateErr(err) {
					return errcode.Conflict
				}
				return err
			}
		} else if err != nil {
			return err
		}
		if target.StockQuantity > quantity.Max-req.Quantity || target.AvailableQty > quantity.Max-req.Quantity {
			return errcode.QuantityLimitExceeded
		}
		fifo := target.StockInTime
		if target.StockQuantity == 0 || source.StockInTime.Before(fifo) {
			fifo = source.StockInTime
		}
		if n, e := s.repo.AdjustNegative(db, source.ID, req.Quantity); e != nil {
			return e
		} else if n == 0 {
			return errcode.MoveInsufficient
		}
		if err = s.repo.IncreaseQty(db, target.ID, req.Quantity, req.Quantity); err != nil {
			return err
		}
		if err = s.repo.SetStockInTime(db, target.ID, fifo); err != nil {
			return err
		}
		id := snowflake.Next()
		no := fmt.Sprintf("YK%s%d", time.Now().Format("20060102"), id)
		for _, entry := range []model.InventoryTrans{
			{ID: snowflake.Next(), InventoryID: source.ID, FrozenBefore: source.FrozenQty, FrozenAfter: source.FrozenQty, TransType: model.TransMoveOut, QuantityChange: -req.Quantity, BeforeQuantity: source.StockQuantity, AfterQuantity: source.StockQuantity - req.Quantity, AvailableBefore: source.AvailableQty, AvailableAfter: source.AvailableQty - req.Quantity, OrderNo: no, Operator: operator},
			{ID: snowflake.Next(), InventoryID: target.ID, FrozenBefore: target.FrozenQty, FrozenAfter: target.FrozenQty, TransType: model.TransMoveIn, QuantityChange: req.Quantity, BeforeQuantity: target.StockQuantity, AfterQuantity: target.StockQuantity + req.Quantity, AvailableBefore: target.AvailableQty, AvailableAfter: target.AvailableQty + req.Quantity, OrderNo: no, Operator: operator},
		} {
			if err = s.repo.InsertTrans(db, &entry); err != nil {
				return err
			}
		}
		if err = s.repo.SyncMoveLocations(db, source.LocationID, req.TargetLocationID); err != nil {
			return err
		}
		from, to := "", ""
		for _, l := range locations {
			if l.ID == source.LocationID {
				from = l.Code
			}
			if l.ID == req.TargetLocationID {
				to = l.Code
			}
		}
		m := &model.Movement{ID: id, OperationKey: key, RequestHash: hash.String(), MoveNo: no, WarehouseID: w.ID, WarehouseCode: w.Code, SourceLocationID: source.LocationID, TargetLocationID: req.TargetLocationID, SourceInventoryID: source.ID, TargetInventoryID: target.ID, SourceLocationCode: from, TargetLocationCode: to, SKUID: sku.ID, SKUCode: sku.Code, SKUName: sku.Name, Unit: sku.Unit, BatchNo: source.BatchNo, Quantity: req.Quantity, StockInTime: source.StockInTime, Operator: operator, Reason: reason, CreatedAt: time.Now().Truncate(time.Millisecond)}
		if err = s.repo.SaveMovement(db, m); err != nil {
			return err
		}
		result = movementResponse(m)
		return nil
	})
	if err == nil {
		return result, nil
	}
	// Any failed candidate has rolled back before borrowing a fresh connection.
	if ctx.Err() != nil {
		return nil, errcode.Wrap(ctx.Err(), errcode.IdempotencyResultUnknown)
	}
	old, findErr := s.repo.FindMovement(s.tm.DB().WithContext(ctx), key)
	if findErr != nil {
		return nil, errcode.Wrap(findErr, errcode.IdempotencyResultUnknown)
	}
	if old != nil {
		return replayMove(old, hash)
	}
	return nil, err
}

func (s *Service) ListMovements(ctx context.Context, q *dto.MoveQuery) ([]*dto.Movement, int64, error) {
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	rows, total, err := s.repo.ListMovements(s.tm.DB().WithContext(ctx), q)
	if err != nil {
		return nil, 0, err
	}
	list := make([]*dto.Movement, 0, len(rows))
	for i := range rows {
		list = append(list, movementResponse(&rows[i]))
	}
	return list, total, nil
}

func (s *Service) MoveLocations(ctx context.Context, q *dto.MoveLocationQuery) ([]*dto.AlertOption, int64, error) {
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	db := s.tm.DB().WithContext(ctx)
	source, err := s.repo.MovementSource(db, q.SourceInventoryID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, 0, errcode.InventoryNotFound
	}
	if err != nil {
		return nil, 0, err
	}
	return s.repo.MoveLocations(db, source, q)
}

func (s *Service) MoveSource(ctx context.Context, id int64) (*dto.MoveSource, error) {
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	return s.repo.MoveSource(s.tm.DB().WithContext(ctx), id)
}
