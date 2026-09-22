package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/bootstrap"
	"gowms/internal/modules/basic/dto"
	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/basic/repository"
	inventoryapi "gowms/internal/modules/inventory/api"
	inventorymodel "gowms/internal/modules/inventory/model"
	inventoryrepository "gowms/internal/modules/inventory/repository"
	inventoryservice "gowms/internal/modules/inventory/service"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
	pkgtx "gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func newDeleteTestService(t *testing.T) (*Service, *inventoryservice.Service, *pkgtx.Manager, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn)
	if err := bootstrap.AutoMigrate(db); err != nil {
		t.Fatalf("migrate isolated database: %v", err)
	}
	if err := snowflake.Init(1); err != nil {
		t.Fatalf("init snowflake: %v", err)
	}

	tm := pkgtx.New(db)
	inventorySvc := inventoryservice.New(inventoryrepository.New(), tm)
	return New(repository.New(), tm, nil, inventorySvc, config.LimitsConfig{}), inventorySvc, tm, db
}

func createDeleteFixtures(t *testing.T, db *gorm.DB) (warehouseID, locationID, skuID int64) {
	t.Helper()

	warehouseID = snowflake.Next()
	locationID = snowflake.Next()
	skuID = snowflake.Next()
	if err := db.Create(&basicmodel.Warehouse{
		Base: sysmodel.Base{ID: warehouseID}, Code: fmt.Sprintf("WH-%d", warehouseID),
		Name: "测试仓库", Status: 1,
	}).Error; err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locationID}, WarehouseID: warehouseID,
		Code: fmt.Sprintf("LOC-%d", locationID), Zone: "T", Status: basicmodel.LocationStatusIdle,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	if err := db.Create(&basicmodel.SKU{
		Base: sysmodel.Base{ID: skuID}, Code: fmt.Sprintf("SKU-%d", skuID),
		Barcode: fmt.Sprintf("BAR-%d", skuID), Name: "测试货品", Unit: "件", Status: 1,
	}).Error; err != nil {
		t.Fatalf("create sku: %v", err)
	}
	return warehouseID, locationID, skuID
}

func addInventory(t *testing.T, tm *pkgtx.Manager, inventorySvc *inventoryservice.Service, warehouseID, locationID, skuID int64) {
	t.Helper()
	err := tm.Tx(context.Background(), func(txDB *gorm.DB) error {
		return inventorySvc.Increase(context.Background(), txDB, &inventoryapi.IncreaseReq{
			WarehouseID: warehouseID, LocationID: locationID, SKUID: skuID,
			BatchNo: "B-DELETE-TEST", Quantity: 1, OrderNo: "TEST", Operator: "test",
		})
	})
	if err != nil {
		t.Fatalf("add inventory: %v", err)
	}
}

func TestDeleteBasicDataReturnsNotFound(t *testing.T) {
	svc, _, _, _ := newDeleteTestService(t)
	ctx := context.Background()

	if err := svc.DeleteWarehouse(ctx, snowflake.Next()); !errors.Is(err, errcode.WarehouseNotFound) {
		t.Fatalf("missing warehouse error = %v", err)
	}
	if err := svc.DeleteLocation(ctx, snowflake.Next()); !errors.Is(err, errcode.LocationNotFound) {
		t.Fatalf("missing location error = %v", err)
	}
	if err := svc.DeleteSKU(ctx, snowflake.Next()); !errors.Is(err, errcode.SKUNotFound) {
		t.Fatalf("missing sku error = %v", err)
	}
}

func TestDeleteBasicDataBlocksInventoryAndReferences(t *testing.T) {
	svc, inventorySvc, tm, db := newDeleteTestService(t)
	ctx := context.Background()
	warehouseID, locationID, skuID := createDeleteFixtures(t, db)

	if err := svc.DeleteWarehouse(ctx, warehouseID); !errors.Is(err, errcode.WarehouseHasReferences) {
		t.Fatalf("warehouse with location error = %v", err)
	}
	addInventory(t, tm, inventorySvc, warehouseID, locationID, skuID)

	if err := svc.DeleteWarehouse(ctx, warehouseID); !errors.Is(err, errcode.WarehouseHasStock) {
		t.Fatalf("warehouse with inventory error = %v", err)
	}
	if err := svc.DeleteLocation(ctx, locationID); !errors.Is(err, errcode.LocationHasStock) {
		t.Fatalf("location with inventory error = %v", err)
	}
	if err := svc.DeleteSKU(ctx, skuID); !errors.Is(err, errcode.SKUHasStock) {
		t.Fatalf("sku with inventory error = %v", err)
	}
}

func TestDeleteBasicDataRejectsZeroStockInventoryHistory(t *testing.T) {
	svc, _, _, db := newDeleteTestService(t)
	ctx := context.Background()
	warehouseID, locationID, skuID := createDeleteFixtures(t, db)
	if err := db.Create(&inventorymodel.Inventory{
		TenantID: 0, WarehouseID: warehouseID, LocationID: locationID, SKUID: skuID,
		BatchNo: "ZERO", StockQuantity: 0, AvailableQty: 0, AllocatedQty: 0, StockInTime: time.Now(),
	}).Error; err != nil {
		t.Fatalf("create zero stock inventory: %v", err)
	}

	if err := svc.DeleteWarehouse(ctx, warehouseID); !errors.Is(err, errcode.WarehouseHasStock) {
		t.Fatalf("warehouse zero-stock history error = %v", err)
	}
	if err := svc.DeleteLocation(ctx, locationID); !errors.Is(err, errcode.LocationHasStock) {
		t.Fatalf("location zero-stock history error = %v", err)
	}
	if err := svc.DeleteSKU(ctx, skuID); !errors.Is(err, errcode.SKUHasStock) {
		t.Fatalf("sku zero-stock history error = %v", err)
	}
}

func TestConcurrentDeleteSKUAndIncreaseDoesNotCreateOrphanInventory(t *testing.T) {
	svc, inventorySvc, tm, db := newDeleteTestService(t)
	ctx := context.Background()
	warehouseID, locationID, skuID := createDeleteFixtures(t, db)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var deleteErr, increaseErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		deleteErr = svc.DeleteSKU(ctx, skuID)
	}()
	go func() {
		defer wg.Done()
		<-start
		increaseErr = tm.Tx(ctx, func(txDB *gorm.DB) error {
			return inventorySvc.Increase(ctx, txDB, &inventoryapi.IncreaseReq{
				WarehouseID: warehouseID, LocationID: locationID, SKUID: skuID,
				BatchNo: "B-RACE-SKU", Quantity: 1, OrderNo: "RACE", Operator: "test",
			})
		})
	}()
	close(start)
	wg.Wait()

	if deleteErr == nil && increaseErr == nil {
		t.Fatal("delete sku and increase inventory both succeeded")
	}
	var inventoryCount int64
	if err := db.Table("wms_inventory").Where("sku_id = ?", skuID).Count(&inventoryCount).Error; err != nil {
		t.Fatalf("count inventory: %v", err)
	}
	if inventoryCount > 0 {
		var sku basicmodel.SKU
		if err := db.First(&sku, skuID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("inventory was created for a deleted sku")
		}
	}
}

func TestConcurrentDeleteLocationAndIncreaseDoesNotCreateOrphanInventory(t *testing.T) {
	svc, inventorySvc, tm, db := newDeleteTestService(t)
	ctx := context.Background()
	warehouseID, locationID, skuID := createDeleteFixtures(t, db)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var deleteErr, increaseErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		deleteErr = svc.DeleteLocation(ctx, locationID)
	}()
	go func() {
		defer wg.Done()
		<-start
		increaseErr = tm.Tx(ctx, func(txDB *gorm.DB) error {
			return inventorySvc.Increase(ctx, txDB, &inventoryapi.IncreaseReq{
				WarehouseID: warehouseID, LocationID: locationID, SKUID: skuID,
				BatchNo: "B-RACE-LOC", Quantity: 1, OrderNo: "RACE", Operator: "test",
			})
		})
	}()
	close(start)
	wg.Wait()

	if deleteErr == nil && increaseErr == nil {
		t.Fatal("delete location and increase inventory both succeeded")
	}
	var inventoryCount int64
	if err := db.Table("wms_inventory").Where("location_id = ?", locationID).Count(&inventoryCount).Error; err != nil {
		t.Fatalf("count inventory: %v", err)
	}
	if inventoryCount > 0 {
		var location basicmodel.Location
		if err := db.First(&location, locationID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("inventory was created for a deleted location")
		}
	}
}

func TestConcurrentDeleteWarehouseAndCreateLocationDoesNotCreateOrphanLocation(t *testing.T) {
	svc, _, _, db := newDeleteTestService(t)
	ctx := context.Background()
	warehouseID := snowflake.Next()
	if err := db.Create(&basicmodel.Warehouse{
		Base: sysmodel.Base{ID: warehouseID}, Code: fmt.Sprintf("WH-%d", warehouseID), Name: "并发仓库", Status: 1,
	}).Error; err != nil {
		t.Fatalf("create warehouse: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var deleteErr, createErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		deleteErr = svc.DeleteWarehouse(ctx, warehouseID)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, createErr = svc.BatchCreateLocations(ctx, &dto.LocationBatchReq{
			WarehouseID: warehouseID, Zone: "RACE", RowFrom: 1, RowTo: 1, ColFrom: 1, ColTo: 1,
		})
	}()
	close(start)
	wg.Wait()

	if deleteErr == nil && createErr == nil {
		t.Fatal("delete warehouse and create location both succeeded")
	}
	var locationCount int64
	if err := db.Table("wms_location").Where("warehouse_id = ?", warehouseID).Count(&locationCount).Error; err != nil {
		t.Fatalf("count locations: %v", err)
	}
	if locationCount > 0 {
		var warehouse basicmodel.Warehouse
		if err := db.First(&warehouse, warehouseID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatal("location was created for a deleted warehouse")
		}
	}
}
