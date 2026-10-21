package service

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

func scaleReference(ctx context.Context, t *testing.T, db *gorm.DB, warehouseID int64, label string) (int64, int64) {
	t.Helper()
	skuID, locationID := snowflake.Next(), snowflake.Next()
	for _, row := range []any{
		&basicmodel.SKU{Base: modelbase.Base{ID: skuID}, Code: "SKU-" + label, Barcode: "BC-" + label, Name: "scale", Status: 1},
		&basicmodel.Location{Base: modelbase.Base{ID: locationID}, WarehouseID: warehouseID, Code: "LOC-" + label, Status: 1},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return locationID, skuID
}

func scaleWarehouse(ctx context.Context, t *testing.T, db *gorm.DB, label string) int64 {
	t.Helper()
	warehouseID := snowflake.Next()
	if err := db.WithContext(ctx).Create(&basicmodel.Warehouse{Base: modelbase.Base{ID: warehouseID}, Code: "WH-" + label, Name: "scale", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	return warehouseID
}

// The same warehouse row serializes even disjoint SKU/location tuples. Confirm
// the real MySQL wait instead of inferring contention from goroutine scheduling.
func TestIncreaseWarehouseGate(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 73), 8*time.Second)
	defer cancel()
	first := scaleWarehouse(ctx, t, db, "held")
	second := scaleWarehouse(ctx, t, db, "independent")
	location, sku := scaleReference(ctx, t, db, first, "blocked")
	otherLocation, otherSKU := scaleReference(ctx, t, db, second, "independent")
	holder := db.WithContext(ctx).Begin()
	if holder.Error != nil {
		t.Fatal(holder.Error)
	}
	defer func() { _ = holder.Rollback().Error }()
	if err := holder.Exec("SELECT id FROM wms_warehouse WHERE id = ? FOR UPDATE", first).Error; err != nil {
		t.Fatal(err)
	}
	sameDone := make(chan error, 1)
	received := false
	defer func() {
		cancel()
		_ = holder.Rollback().Error
		if !received {
			select {
			case <-sameDone:
			case <-time.After(3 * time.Second):
				t.Error("blocked Increase goroutine did not stop")
			}
		}
	}()
	go func() {
		sameDone <- tm.Tx(ctx, func(tx *gorm.DB) error {
			return svc.Increase(ctx, tx, &api.IncreaseReq{WarehouseID: first, LocationID: location, SKUID: sku, BatchNo: "LOCK", Quantity: 1})
		})
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	started := time.Now()
	for {
		var waiting int64
		query := "SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks r ON w.REQUESTING_ENGINE_LOCK_ID = r.ENGINE_LOCK_ID WHERE r.OBJECT_SCHEMA = DATABASE() AND r.OBJECT_NAME = 'wms_warehouse'"
		if err := db.WithContext(ctx).Raw(query).Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case err := <-sameDone:
			received = true
			t.Fatalf("same warehouse bypassed held row: %v", err)
		case <-ctx.Done():
			t.Fatal("warehouse wait was not observed")
		case <-ticker.C:
		}
	}
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Increase(ctx, tx, &api.IncreaseReq{WarehouseID: second, LocationID: otherLocation, SKUID: otherSKU, BatchNo: "FREE", Quantity: 1})
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-sameDone:
		received = true
		t.Fatalf("same warehouse finished before release: %v", err)
	default:
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-sameDone:
		received = true
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Increase did not resume after warehouse release")
	}
	var stocks, flows int64
	if err := db.WithContext(ctx).Model(&model.Inventory{}).Count(&stocks).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&model.InventoryTrans{}).Count(&flows).Error; err != nil {
		t.Fatal(err)
	}
	if stocks != 2 || flows != 2 {
		t.Fatalf("stocks=%d flows=%d", stocks, flows)
	}
	t.Logf("WAREHOUSE_GATE observed_wait=true independent_warehouse_completed=true elapsed_ms=%.3f", float64(time.Since(started).Microseconds())/1000)
}

func TestIncreaseWarehouseContentionProfile(t *testing.T) {
	for _, sameWarehouse := range []bool{true, false} {
		t.Run(fmt.Sprintf("shared_warehouse_%v", sameWarehouse), func(t *testing.T) {
			svc, tm, db := newTestService(t)
			ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 74), 15*time.Second)
			defer cancel()
			shared := scaleWarehouse(ctx, t, db, "shared")
			requests := make([]api.IncreaseReq, 8)
			for i := range requests {
				warehouseID := shared
				if !sameWarehouse {
					warehouseID = scaleWarehouse(ctx, t, db, fmt.Sprint(i))
				}
				locationID, skuID := scaleReference(ctx, t, db, warehouseID, fmt.Sprint(i))
				// Pre-existing tuples isolate the warehouse gate from concurrent
				// missing-row insert gap locks (which can independently deadlock).
				if err := db.WithContext(ctx).Create(&model.Inventory{Base: modelbase.Base{ID: snowflake.Next()}, WarehouseID: warehouseID, LocationID: locationID, SKUID: skuID, BatchNo: "PROFILE", StockQuantity: 1, AvailableQty: 1, StockInTime: time.Now()}).Error; err != nil {
					t.Fatal(err)
				}
				requests[i] = api.IncreaseReq{WarehouseID: warehouseID, LocationID: locationID, SKUID: skuID, BatchNo: "PROFILE", Quantity: 1}
			}
			type outcome struct {
				elapsed time.Duration
				err     error
			}
			done := make(chan outcome, len(requests))
			start := make(chan struct{})
			begin := time.Now()
			for _, request := range requests {
				go func() {
					<-start
					started := time.Now()
					err := tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(tx *gorm.DB) error { return svc.Increase(ctx, tx, &request) })
					done <- outcome{time.Since(started), err}
				}()
			}
			close(start)
			times := make([]float64, 0, len(requests))
			for range requests {
				result := <-done
				if result.err != nil {
					t.Error(result.err)
				}
				times = append(times, float64(result.elapsed.Microseconds())/1000)
			}
			sort.Float64s(times)
			var count int64
			if err := db.WithContext(ctx).Model(&model.Inventory{}).Where("stock_quantity = 2 AND available_quantity = 2 AND allocated_quantity = 0").Count(&count).Error; err != nil || count != 8 {
				t.Fatalf("committed tuples=%d err=%v", count, err)
			}
			if err := db.WithContext(ctx).Model(&model.InventoryTrans{}).Count(&count).Error; err != nil || count != 8 {
				t.Fatalf("flows=%d err=%v", count, err)
			}
			t.Logf("INCREASE_PROFILE shared_warehouse=%v workers=8 elapsed_ms=%.3f median_ms=%.3f max_ms=%.3f", sameWarehouse, float64(time.Since(begin).Microseconds())/1000, times[len(times)/2], times[len(times)-1])
		})
	}
}
