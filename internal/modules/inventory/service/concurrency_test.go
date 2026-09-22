package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/api"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
	pkgtx "gowms/internal/pkg/tx"
)

func prepareAllocatedStock(t *testing.T, svc *Service, tm *pkgtx.Manager, db *gorm.DB, quantity int) (inventoryID int64) {
	t.Helper()
	ctx := context.Background()
	locationID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locationID}, WarehouseID: 1,
		Code: fmt.Sprintf("T-%d", locationID), Status: basicmodel.LocationStatusIdle,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	warehouseID := setupStock(t, svc, tm, locationID, skuID, "B-CONC", quantity)
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		result, err := svc.Allocate(ctx, tx, &api.AllocateReq{
			WarehouseID: warehouseID, SKUID: skuID, Quantity: quantity,
			OrderNo: "CK-CONC", Operator: "test",
		})
		if err != nil {
			return err
		}
		if len(result.Rows) != 1 {
			return fmt.Errorf("expected one allocation row, got %d", len(result.Rows))
		}
		inventoryID = result.Rows[0].InventoryID
		return nil
	}); err != nil {
		t.Fatalf("allocate stock: %v", err)
	}
	return inventoryID
}

func TestConcurrentReleaseDoesNotDoubleRelease(t *testing.T) {
	svc, tm, db := newTestService(t)
	inventoryID := prepareAllocatedStock(t, svc, tm, db, 10)
	ctx := context.Background()

	start := make(chan struct{})
	var success, rejected atomic.Int64
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := tm.Tx(ctx, func(tx *gorm.DB) error {
				return svc.Release(ctx, tx, &api.ReleaseReq{
					InventoryID: inventoryID, Quantity: 10, OrderNo: "CK-CONC", Operator: "test",
				})
			})
			if err == nil {
				success.Add(1)
				return
			}
			if errcode.From(err).Code == errcode.AllocatedNotEnough.Code {
				rejected.Add(1)
				return
			}
			t.Errorf("release: %v", err)
		}()
	}
	close(start)
	wg.Wait()

	if success.Load() != 1 || rejected.Load() != 1 {
		t.Fatalf("release success=%d rejected=%d", success.Load(), rejected.Load())
	}
	var inventory struct {
		StockQuantity int
		AvailableQty  int
		AllocatedQty  int
	}
	if err := db.Table("wms_inventory").Where("id = ?", inventoryID).
		Select("stock_quantity, available_quantity AS available_qty, allocated_quantity AS allocated_qty").
		Scan(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.StockQuantity != inventory.AvailableQty+inventory.AllocatedQty ||
		inventory.AvailableQty < 0 || inventory.AllocatedQty < 0 {
		t.Fatalf("invariant broken: %+v", inventory)
	}
}

func TestConcurrentShipDoesNotDoubleDeduct(t *testing.T) {
	svc, tm, db := newTestService(t)
	inventoryID := prepareAllocatedStock(t, svc, tm, db, 10)
	ctx := context.Background()

	start := make(chan struct{})
	var success, rejected atomic.Int64
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := tm.Tx(ctx, func(tx *gorm.DB) error {
				return svc.Ship(ctx, tx, &api.ShipReq{
					InventoryID: inventoryID, Quantity: 10, OrderNo: "CK-CONC", Operator: "test",
				})
			})
			if err == nil {
				success.Add(1)
				return
			}
			if errcode.From(err).Code == errcode.ShipConflict.Code {
				rejected.Add(1)
				return
			}
			t.Errorf("ship: %v", err)
		}()
	}
	close(start)
	wg.Wait()

	if success.Load() != 1 || rejected.Load() != 1 {
		t.Fatalf("ship success=%d rejected=%d", success.Load(), rejected.Load())
	}
	var inventory struct {
		StockQuantity int
		AvailableQty  int
		AllocatedQty  int
	}
	if err := db.Table("wms_inventory").Where("id = ?", inventoryID).
		Select("stock_quantity, available_quantity AS available_qty, allocated_quantity AS allocated_qty").
		Scan(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.StockQuantity != inventory.AvailableQty+inventory.AllocatedQty ||
		inventory.StockQuantity < 0 || inventory.AvailableQty < 0 || inventory.AllocatedQty < 0 {
		t.Fatalf("invariant broken: %+v", inventory)
	}
}
