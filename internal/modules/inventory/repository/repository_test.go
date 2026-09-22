package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func inventoryFixture(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.Inventory{}, &basicmodel.SKU{}, &basicmodel.Location{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestInventoryMutationsCannotCrossTenantOrTouchDeletedRows(t *testing.T) {
	db := inventoryFixture(t)
	r := New()
	for _, row := range []*model.Inventory{
		{Base: sysmodel.Base{ID: 1}, TenantID: 22, WarehouseID: 1, LocationID: 1, SKUID: 1, StockQuantity: 10, AvailableQty: 8, AllocatedQty: 2},
		{Base: sysmodel.Base{ID: 2, DeletedAt: gorm.DeletedAt{Time: time.Now(), Valid: true}}, TenantID: 11, WarehouseID: 1, LocationID: 2, SKUID: 1, StockQuantity: 10, AvailableQty: 8, AllocatedQty: 2},
	} {
		row.StockInTime = time.Now()
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	owned := db.WithContext(tenant.WithTenant(context.Background(), 11))
	for _, id := range []int64{1, 2} {
		for _, mutate := range []func(*gorm.DB, int64, int) (int64, error){r.AllocateQty, r.ShipQty, r.ReleaseQty, r.AdjustNegative} {
			if rows, err := mutate(owned, id, 1); err != nil || rows != 0 {
				t.Fatalf("unauthorized mutation: rows=%d err=%v", rows, err)
			}
		}
		if err := r.IncreaseQty(owned, id, 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := r.AdjustPositive(owned, id, 1); err != nil {
			t.Fatal(err)
		}
		var got model.Inventory
		if err := db.Unscoped().First(&got, id).Error; err != nil {
			t.Fatal(err)
		}
		if got.StockQuantity != 10 || got.AvailableQty != 8 || got.AllocatedQty != 2 {
			t.Fatalf("row changed: %+v", got)
		}
	}
}

func TestSummaryCountsSKUsAndFIFOFiltersTenant(t *testing.T) {
	db := inventoryFixture(t)
	for _, row := range []any{
		&basicmodel.SKU{Base: sysmodel.Base{ID: 1}, TenantID: 11, Code: "A", Barcode: "A", Name: "A"},
		&basicmodel.SKU{Base: sysmodel.Base{ID: 2}, TenantID: 11, Code: "B", Barcode: "B", Name: "B"},
		&basicmodel.Location{Base: sysmodel.Base{ID: 1}, TenantID: 11, Code: "L1", WarehouseID: 1},
		&basicmodel.Location{Base: sysmodel.Base{ID: 2}, TenantID: 22, Code: "L2", WarehouseID: 1},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []*model.Inventory{
		{Base: sysmodel.Base{ID: 1}, TenantID: 11, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "A", StockQuantity: 10, AvailableQty: 10, StockInTime: time.Now().Add(-time.Hour)},
		{Base: sysmodel.Base{ID: 2}, TenantID: 11, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B", StockQuantity: 20, AvailableQty: 20, StockInTime: time.Now()},
		{Base: sysmodel.Base{ID: 3}, TenantID: 11, WarehouseID: 1, LocationID: 1, SKUID: 2, BatchNo: "A", StockQuantity: 1, AvailableQty: 1, StockInTime: time.Now()},
		{Base: sysmodel.Base{ID: 4}, TenantID: 22, WarehouseID: 1, LocationID: 2, SKUID: 1, BatchNo: "A", StockQuantity: 99, AvailableQty: 99, StockInTime: time.Now().Add(-2 * time.Hour)},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := tenant.WithTenant(context.Background(), 11)
	r := New()
	list, total, err := r.SummaryBySKU(ctx, db, 1, 1, 1)
	if err != nil || total != 2 || len(list) != 1 {
		t.Fatalf("summary=%v total=%d err=%v", list, total, err)
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := r.FindFIFOForUpdate(tx, 1, 1)
		if err != nil {
			return err
		}
		if len(rows) != 2 || rows[0].ID != 1 || rows[1].ID != 2 {
			t.Fatalf("FIFO crossed tenant or order changed: %+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestRawInventoryQueriesRespectExactTenantZero(t *testing.T) {
	db := inventoryFixture(t)
	for _, row := range []any{
		&basicmodel.SKU{Base: sysmodel.Base{ID: 1}, TenantID: 0, Code: "P", Barcode: "P", Name: "平台货品"},
		&basicmodel.Location{Base: sysmodel.Base{ID: 1}, TenantID: 0, Code: "P-L", WarehouseID: 1},
		&basicmodel.Location{Base: sysmodel.Base{ID: 2}, TenantID: 11, Code: "T-L", WarehouseID: 1},
	} {
		if err := db.WithContext(context.Background()).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []*model.Inventory{
		{Base: sysmodel.Base{ID: 1}, TenantID: 0, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "P", StockQuantity: 10, AvailableQty: 10, StockInTime: time.Now()},
		{Base: sysmodel.Base{ID: 2}, TenantID: 11, WarehouseID: 1, LocationID: 2, SKUID: 1, BatchNo: "T", StockQuantity: 99, AvailableQty: 99, StockInTime: time.Now()},
	} {
		if err := db.WithContext(context.Background()).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	ctx := tenant.WithExactTenant(context.Background(), 0)
	r := New()
	_, total, err := r.SummaryBySKU(ctx, db, 1, 1, 10)
	if err != nil || total != 1 {
		t.Fatalf("exact tenant 0 summary total=%d err=%v", total, err)
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		rows, err := r.FindFIFOForUpdate(tx, 1, 1)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].TenantID != 0 {
			t.Fatalf("exact tenant 0 FIFO rows=%+v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestListSKUKeywordFiltersTenant(t *testing.T) {
	db := inventoryFixture(t)
	for _, row := range []any{
		&basicmodel.SKU{Base: sysmodel.Base{ID: 10}, TenantID: 11, Code: "MATCH-A", Barcode: "MATCH-A", Name: "匹配货品"},
		&basicmodel.SKU{Base: sysmodel.Base{ID: 20}, TenantID: 22, Code: "MATCH-B", Barcode: "MATCH-B", Name: "匹配货品"},
		&basicmodel.Location{Base: sysmodel.Base{ID: 10}, TenantID: 11, Code: "L-10", WarehouseID: 1},
		&basicmodel.Location{Base: sysmodel.Base{ID: 20}, TenantID: 22, Code: "L-20", WarehouseID: 1},
	} {
		if err := db.WithContext(context.Background()).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []*model.Inventory{
		{Base: sysmodel.Base{ID: 10}, TenantID: 11, WarehouseID: 1, LocationID: 10, SKUID: 10, BatchNo: "A", StockQuantity: 1, AvailableQty: 1, StockInTime: time.Now()},
		{Base: sysmodel.Base{ID: 20}, TenantID: 22, WarehouseID: 1, LocationID: 20, SKUID: 20, BatchNo: "B", StockQuantity: 1, AvailableQty: 1, StockInTime: time.Now()},
	} {
		if err := db.WithContext(context.Background()).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	ctx := tenant.WithTenant(context.Background(), 11)
	list, total, err := New().List(ctx, db, &QueryFilter{SKUKeyword: "MATCH", Page: 1, Size: 10})
	if err != nil || total != 1 || len(list) != 1 || list[0].TenantID != 11 {
		t.Fatalf("keyword list crossed tenant: total=%d list=%+v err=%v", total, list, err)
	}
}
