package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

func TestSharedStockReferencesStillSerializeCatalogDeletion(t *testing.T) {
	for _, catalog := range []string{"warehouse", "sku"} {
		t.Run(catalog, func(t *testing.T) {
			svc, tm, db := newTestService(t)
			ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 82), 8*time.Second)
			defer cancel()
			warehouse := scaleWarehouse(ctx, t, db, "protected")
			location, sku := scaleReference(ctx, t, db, warehouse, "protected")
			holder := db.WithContext(ctx).Begin()
			defer holder.Rollback()
			if err := svc.Increase(ctx, holder, &api.IncreaseReq{WarehouseID: warehouse, LocationID: location, SKUID: sku, BatchNo: "PROTECTED", Quantity: 1}); err != nil {
				t.Fatal(err)
			}
			basic := basicservice.New(basicrepo.New(), tm, nil, svc, config.LimitsConfig{})
			done := make(chan error, 1)
			received := false
			defer func() {
				cancel()
				_ = holder.Rollback().Error
				if !received {
					<-done
				}
			}()
			go func() {
				if catalog == "warehouse" {
					done <- basic.DeleteWarehouse(ctx, warehouse)
				} else {
					done <- basic.DeleteSKU(ctx, sku)
				}
			}()
			for {
				var waiting int64
				if err := db.WithContext(ctx).Raw("SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks r ON w.REQUESTING_ENGINE_LOCK_ID=r.ENGINE_LOCK_ID WHERE r.OBJECT_SCHEMA=DATABASE() AND r.OBJECT_NAME=?", "wms_"+catalog).Scan(&waiting).Error; err != nil {
					t.Fatal(err)
				}
				if waiting > 0 {
					break
				}
				select {
				case err := <-done:
					received = true
					t.Fatalf("catalog delete bypassed stock transaction: %v", err)
				case <-ctx.Done():
					t.Fatal("catalog wait was not observed")
				case <-time.After(5 * time.Millisecond):
				}
			}
			if err := holder.Commit().Error; err != nil {
				t.Fatal(err)
			}
			err := <-done
			received = true
			want := errcode.WarehouseHasStock
			if catalog == "sku" {
				want = errcode.SKUHasStock
			}
			if err == nil || errcode.From(err).Code != want.Code {
				t.Fatalf("catalog delete after stock commit: %v", err)
			}
		})
	}
}

func TestIndependentInventoryReferencesCanProceedBeforeFirstCommit(t *testing.T) {
	for _, sameWarehouse := range []bool{true, false} {
		t.Run(map[bool]string{true: "same warehouse distinct bins same SKU", false: "distinct warehouses same SKU"}[sameWarehouse], func(t *testing.T) {
			svc, tm, db := newTestService(t)
			ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 81), 8*time.Second)
			defer cancel()
			first := scaleWarehouse(ctx, t, db, "parallel-first")
			second := first
			if !sameWarehouse {
				second = scaleWarehouse(ctx, t, db, "parallel-second")
			}
			location, sku := scaleReference(ctx, t, db, first, "parallel")
			otherLocation := snowflake.Next()
			if err := db.WithContext(ctx).Create(&basicmodel.Location{Base: modelbase.Base{ID: otherLocation}, WarehouseID: second, Code: "OTHER", Status: 1}).Error; err != nil {
				t.Fatal(err)
			}
			requests := []api.IncreaseReq{
				{WarehouseID: first, LocationID: location, SKUID: sku, BatchNo: "PARALLEL", Quantity: 1},
				{WarehouseID: second, LocationID: otherLocation, SKUID: sku, BatchNo: "PARALLEL", Quantity: 1},
			}
			// Existing tuples isolate catalog contention from missing-tuple gap locks.
			for _, req := range requests {
				if err := db.WithContext(ctx).Create(&model.Inventory{Base: modelbase.Base{ID: snowflake.Next()}, WarehouseID: req.WarehouseID, LocationID: req.LocationID, SKUID: sku, BatchNo: req.BatchNo, StockQuantity: 1, AvailableQty: 1, StockInTime: time.Now()}).Error; err != nil {
					t.Fatal(err)
				}
			}
			holder := db.WithContext(ctx).Begin()
			defer holder.Rollback()
			if err := svc.Increase(ctx, holder, &requests[0]); err != nil {
				t.Fatal(err)
			}
			// This must complete while the first transaction still holds its references.
			if err := tm.Tx(ctx, func(tx *gorm.DB) error { return svc.Increase(ctx, tx, &requests[1]) }); err != nil {
				t.Fatalf("independent stock was serialized behind catalog references: %v", err)
			}
			if err := holder.Commit().Error; err != nil {
				t.Fatal(err)
			}
			var count int64
			if err := db.WithContext(ctx).Model(&model.Inventory{}).Where("stock_quantity=2 AND available_quantity=2").Count(&count).Error; err != nil || count != 2 {
				t.Fatalf("committed stocks=%d err=%v", count, err)
			}
			if err := db.WithContext(ctx).Model(&model.InventoryTrans{}).Count(&count).Error; err != nil || count != 2 {
				t.Fatalf("committed ledger=%d err=%v", count, err)
			}
		})
	}
}
