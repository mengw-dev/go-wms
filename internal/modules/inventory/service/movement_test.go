package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/quantity"
	"gowms/internal/pkg/tenant"
)

func moveFixture(t *testing.T) (*Service, *gorm.DB, context.Context) {
	t.Helper()
	s, _, db := newTestService(t)
	if err := db.AutoMigrate(&model.Movement{}); err != nil {
		t.Fatal(err)
	}
	for _, tid := range []int64{0, 11, 22} {
		q := db.WithContext(tenant.WithExactTenant(context.Background(), tid))
		b := tid * 100
		for _, row := range []any{
			&basicmodel.Warehouse{Base: modelbase.Base{ID: b + 1}, Code: "WH", Name: "仓库", Status: 1},
			&basicmodel.SKU{Base: modelbase.Base{ID: b + 1}, Code: "S%_", Name: "货品", Barcode: "00123", Unit: "箱", Status: 1},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 1}, WarehouseID: b + 1, Code: "A01", Status: 2},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 2}, WarehouseID: b + 1, Code: "A02", Status: 1},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 3}, WarehouseID: b + 1, Code: "A03", Status: 1},
			&model.Inventory{Base: modelbase.Base{ID: b + 10}, WarehouseID: b + 1, LocationID: b + 1, SKUID: b + 1, BatchNo: "B001", StockQuantity: 100, AvailableQty: 80, AllocatedQty: 20, StockInTime: time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)},
		} {
			if err := q.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return s, db, tenant.WithExactTenant(context.Background(), 11)
}
func moveReq() *dto.MoveRequest {
	return &dto.MoveRequest{SourceInventoryID: 1110, TargetLocationID: 1102, Quantity: 30, Reason: "整理库位"}
}

func TestMoveReceiptConservationReplayAndIsolation(t *testing.T) {
	s, db, ctx := moveFixture(t)
	first, err := s.MoveStock(ctx, moveReq(), "move-1", 42, "worker")
	if err != nil {
		t.Fatal(err)
	}
	var source, target model.Inventory
	if err = db.WithContext(ctx).First(&source, 1110).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.WithContext(ctx).First(&target, first.TargetInventoryID).Error; err != nil {
		t.Fatal(err)
	}
	if source.StockQuantity != 70 || source.AvailableQty != 50 || source.AllocatedQty != 20 || target.StockQuantity != 30 || target.AvailableQty != 30 || target.AllocatedQty != 0 || !target.StockInTime.Equal(source.StockInTime) {
		t.Fatalf("source=%+v target=%+v", source, target)
	}
	var entries []model.InventoryTrans
	if err = db.WithContext(ctx).Order("id").Find(&entries).Error; err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].TransType != model.TransMoveOut || entries[1].TransType != model.TransMoveIn || entries[0].QuantityChange+entries[1].QuantityChange != 0 || entries[0].OrderNo != first.MoveNo || entries[1].OrderNo != first.MoveNo {
		t.Fatalf("ledger=%+v", entries)
	}
	// Replay is independent of today's availability and disabled references.
	if err = db.WithContext(ctx).Model(&basicmodel.Warehouse{}).Where("id=1101").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := s.MoveStock(ctx, moveReq(), "move-1", 42, "renamed-worker")
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID || replay.Operator != "worker" || replay.TargetLocationID != 1102 || !replay.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("replay=%+v", replay)
	}
	changed := moveReq()
	changed.Quantity++
	if _, err = s.MoveStock(ctx, changed, "move-1", 42, "worker"); !errors.Is(err, errcode.IdempotencyKeyReused) {
		t.Fatalf("wrong key accepted: %v", err)
	}
	if _, err = s.MoveStock(ctx, moveReq(), "move-1", 43, "other"); !errors.Is(err, errcode.IdempotencyKeyReused) {
		t.Fatalf("actor omitted: %v", err)
	}
	rows, total, err := s.ListMovements(ctx, &dto.MoveQuery{Keyword: "S%_", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("history %d %v", total, err)
	}
	b, _ := json.Marshal(rows)
	if strings.Contains(string(b), "operation_key") || strings.Contains(string(b), "request_hash") {
		t.Fatal("private fields exposed")
	}
	for _, tid := range []int64{0, 22} {
		rows, total, err = s.ListMovements(tenant.WithExactTenant(ctx, tid), &dto.MoveQuery{Page: 1, PageSize: 10})
		if err != nil || total != 0 || len(rows) != 0 {
			t.Fatalf("tenant %d leaked receipts", tid)
		}
	}
	// A plain platform context is deliberately exact tenant zero for moves.
	zero := &dto.MoveRequest{SourceInventoryID: 10, TargetLocationID: 2, Quantity: 80, Reason: "平台租户移库"}
	if _, err = s.MoveStock(context.Background(), zero, "move-1", 42, "worker"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.MoveStock(context.Background(), moveReq(), "foreign", 42, "worker"); !errors.Is(err, errcode.InventoryNotFound) {
		t.Fatalf("platform wrote another tenant: %v", err)
	}
}

func TestMoveValidationMergeFIFOAndLocationOccupancy(t *testing.T) {
	t.Run("rejects", func(t *testing.T) {
		s, db, ctx := moveFixture(t)
		for _, edit := range []func(*dto.MoveRequest){func(q *dto.MoveRequest) { q.Quantity = 81 }, func(q *dto.MoveRequest) { q.Quantity = 0 }, func(q *dto.MoveRequest) { q.Quantity = quantity.Max + 1 }, func(q *dto.MoveRequest) { q.TargetLocationID = 1101 }, func(q *dto.MoveRequest) { q.TargetLocationID = 2202 }, func(q *dto.MoveRequest) { q.Reason = " " }} {
			q := moveReq()
			edit(q)
			if _, err := s.MoveStock(ctx, q, "reject", 42, "w"); err == nil {
				t.Fatalf("accepted %+v", q)
			}
		}
		if err := db.WithContext(ctx).Model(&basicmodel.Location{}).Where("id=1102").Update("warehouse_id", 999).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveStock(ctx, moveReq(), "cross", 42, "w"); !errors.Is(err, errcode.MoveNotAllowed) {
			t.Fatalf("cross warehouse %v", err)
		}
		var n int64
		if err := db.WithContext(ctx).Model(&model.Movement{}).Count(&n).Error; err != nil || n != 0 {
			t.Fatalf("unexpected writes %d %v", n, err)
		}
	})
	t.Run("merge-and-full-free", func(t *testing.T) {
		s, db, ctx := moveFixture(t)
		q := db.WithContext(ctx)
		older := time.Date(2025, 12, 1, 0, 0, 0, 0, time.Local)
		target := &model.Inventory{Base: modelbase.Base{ID: 1120}, WarehouseID: 1101, LocationID: 1102, SKUID: 1101, BatchNo: "B001", StockQuantity: 5, AvailableQty: 5, StockInTime: older}
		if err := q.Create(target).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveStock(ctx, moveReq(), "merge", 42, "w"); err != nil {
			t.Fatal(err)
		}
		if err := q.First(target, 1120).Error; err != nil {
			t.Fatal(err)
		}
		if target.StockQuantity != 35 || !target.StockInTime.Equal(older) {
			t.Fatalf("FIFO reset %+v", target)
		}
		// Source still has allocated stock, so its location stays occupied.
		req := moveReq()
		req.Quantity = 50
		if _, err := s.MoveStock(ctx, req, "remaining", 42, "w"); err != nil {
			t.Fatal(err)
		}
		var location basicmodel.Location
		if err := q.First(&location, 1101).Error; err != nil || location.Status != 2 {
			t.Fatalf("allocated source freed: %+v %v", location, err)
		}
		// A separate completely available batch can empty the location.
		if err := q.Model(&model.Inventory{}).Where("id=1110").Updates(map[string]any{"allocated_quantity": 0, "available_quantity": 20}).Error; err != nil {
			t.Fatal(err)
		}
		req.Quantity = 20
		if _, err := s.MoveStock(ctx, req, "empty", 42, "w"); err != nil {
			t.Fatal(err)
		}
		if err := q.First(&location, 1101).Error; err != nil || location.Status != 1 {
			t.Fatalf("source not idle: %+v %v", location, err)
		}
	})
	t.Run("overflow-rolls-back", func(t *testing.T) {
		s, db, ctx := moveFixture(t)
		if err := db.WithContext(ctx).Create(&model.Inventory{Base: modelbase.Base{ID: 1120}, WarehouseID: 1101, LocationID: 1102, SKUID: 1101, BatchNo: "B001", StockQuantity: quantity.Max, AvailableQty: quantity.Max, StockInTime: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := s.MoveStock(ctx, moveReq(), "overflow", 42, "w"); !errors.Is(err, errcode.QuantityLimitExceeded) {
			t.Fatalf("overflow %v", err)
		}
		var source model.Inventory
		if err := db.WithContext(ctx).First(&source, 1110).Error; err != nil || source.AvailableQty != 80 {
			t.Fatalf("source changed %+v %v", source, err)
		}
	})
}

func TestMoveConcurrentKeysAndLastWriteRollback(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(fmt.Sprint(sameKey), func(t *testing.T) {
			s, db, ctx := moveFixture(t)
			var wg sync.WaitGroup
			results := make(chan error, 8)
			start := make(chan struct{})
			for i := range 8 {
				wg.Go(func() {
					<-start
					key := "same"
					if !sameKey {
						key = fmt.Sprint(i)
					}
					_, err := s.MoveStock(ctx, moveReq(), key, 42, "w")
					results <- err
				})
			}
			close(start)
			wg.Wait()
			close(results)
			success := 0
			for err := range results {
				if err == nil {
					success++
				} else if sameKey || !errors.Is(err, errcode.MoveInsufficient) {
					t.Fatalf("concurrent error %v", err)
				}
			}
			want := 2
			if sameKey {
				want = 8
			}
			if success != want {
				t.Fatalf("success=%d want=%d", success, want)
			}
			var count int64
			wantRows := int64(2)
			if sameKey {
				wantRows = 1
			}
			if err := db.WithContext(ctx).Model(&model.Movement{}).Count(&count).Error; err != nil || count != wantRows {
				t.Fatalf("receipts=%d %v", count, err)
			}
		})
	}
	t.Run("receipt-failure", func(t *testing.T) {
		s, db, ctx := moveFixture(t)
		name := "test:movement-failure"
		if err := db.Callback().Create().Before("gorm:create").Register(name, func(q *gorm.DB) {
			if q.Statement.Table == "wms_inventory_movement" {
				_ = q.AddError(errors.New("injected receipt failure"))
			}
		}); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Callback().Create().Remove(name) })
		if _, err := s.MoveStock(ctx, moveReq(), "rollback", 42, "w"); err == nil {
			t.Fatal("expected failure")
		}
		var source model.Inventory
		if err := db.WithContext(ctx).First(&source, 1110).Error; err != nil || source.StockQuantity != 100 || source.AvailableQty != 80 {
			t.Fatalf("partial stock %+v %v", source, err)
		}
		for _, m := range []any{&model.InventoryTrans{}, &model.Movement{}} {
			var count int64
			if err := db.WithContext(ctx).Model(m).Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("partial audit %T: %d %v", m, count, err)
			}
		}
		var targetCount int64
		if err := db.WithContext(ctx).Model(&model.Inventory{}).Where("location_id=1102").Count(&targetCount).Error; err != nil || targetCount != 0 {
			t.Fatalf("rolled-back target persisted: %d %v", targetCount, err)
		}
		var location basicmodel.Location
		if err := db.WithContext(ctx).First(&location, 1102).Error; err != nil || location.Status != 1 {
			t.Fatalf("rolled-back occupancy persisted: %+v %v", location, err)
		}
	})
}

func TestMoveReferenceAvailabilityAndExactOptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entity  any
		id      int64
		deleted bool
	}{
		{"warehouse-disabled", &basicmodel.Warehouse{}, 1101, false},
		{"source-location-disabled", &basicmodel.Location{}, 1101, false},
		{"target-location-disabled", &basicmodel.Location{}, 1102, false},
		{"sku-disabled", &basicmodel.SKU{}, 1101, false},
		{"warehouse-deleted", &basicmodel.Warehouse{}, 1101, true},
		{"target-location-deleted", &basicmodel.Location{}, 1102, true},
		{"sku-deleted", &basicmodel.SKU{}, 1101, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db, ctx := moveFixture(t)
			q := db.WithContext(ctx).Model(tc.entity).Where("id=?", tc.id)
			var err error
			if tc.deleted {
				err = q.Delete(tc.entity).Error
			} else {
				err = q.Update("status", 0).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.MoveStock(ctx, moveReq(), "disabled", 42, "worker"); err == nil {
				t.Fatal("unavailable reference accepted")
			}
			var inv model.Inventory
			if err = db.WithContext(ctx).First(&inv, 1110).Error; err != nil || inv.StockQuantity != 100 || inv.AvailableQty != 80 {
				t.Fatalf("rejected move changed inventory: %+v %v", inv, err)
			}
		})
	}
	t.Run("options-and-source", func(t *testing.T) {
		s, db, ctx := moveFixture(t)
		source, err := s.MoveSource(ctx, 1110)
		if err != nil || !source.Enabled || source.AvailableQty != 80 || source.SKUCode != "S%_" {
			t.Fatalf("source %+v %v", source, err)
		}
		if _, err = s.MoveSource(context.Background(), 1110); !errors.Is(err, errcode.InventoryNotFound) {
			t.Fatalf("platform source leaked %v", err)
		}
		rows, total, err := s.MoveLocations(ctx, &dto.MoveLocationQuery{SourceInventoryID: 1110, Page: 1, PageSize: 100})
		if err != nil || total != 2 || len(rows) != 2 {
			t.Fatalf("options %+v %d %v", rows, total, err)
		}
		for _, row := range rows {
			if row.ID != 1102 && row.ID != 1103 {
				t.Fatalf("foreign location leaked %+v", row)
			}
		}
		if err = db.WithContext(ctx).Model(&basicmodel.Location{}).Where("id=1102").Update("status", 0).Error; err != nil {
			t.Fatal(err)
		}
		_, total, err = s.MoveLocations(ctx, &dto.MoveLocationQuery{SourceInventoryID: 1110, Page: 1, PageSize: 100})
		if err != nil || total != 1 {
			t.Fatalf("disabled option visible %d %v", total, err)
		}
		_, total, err = s.MoveLocations(ctx, &dto.MoveLocationQuery{SourceInventoryID: 1110, Keyword: "A%", Page: 1, PageSize: 100})
		if err != nil || total != 0 {
			t.Fatalf("LIKE wildcard not escaped %d %v", total, err)
		}
	})
}
