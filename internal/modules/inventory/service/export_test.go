package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/xlsxexport"
)

func readExport(t *testing.T, file *xlsxexport.File) [][]string {
	t.Helper()
	book, err := excelize.OpenReader(bytes.NewReader(file.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	rows, err := book.GetRows("数据")
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestInventoryExportPreservesLargeSKUIdentity(t *testing.T) {
	s, db, ctx := alertFixture(t)
	const skuID = int64(9007199254740993)
	if err := db.WithContext(ctx).Create(&basicmodel.SKU{Base: modelbase.Base{ID: skuID}, Code: "EXPORT-LARGE", Barcode: "000123", Name: "large", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&model.Inventory{Base: modelbase.Base{ID: 9901}, WarehouseID: 1101, LocationID: 1, SKUID: skuID, BatchNo: "EXPORT-ID", StockQuantity: 1, AvailableQty: 1, StockInTime: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	file, err := s.ExportInventory(ctx, &dto.InventoryExportQuery{WarehouseID: 1101})
	if err != nil {
		t.Fatal(err)
	}
	rows := readExport(t, file)
	if len(rows) != 2 || rows[1][6] != "9007199254740993" || rows[1][7] != "EXPORT-LARGE" {
		t.Fatalf("SKU identity changed in workbook: %v", rows)
	}
}

func TestInventoryExportsFilterAllPagesAndExactTenant(t *testing.T) {
	s, db, ctx := alertFixture(t)
	for _, tid := range []int64{0, 11, 22} {
		rows := make([]*model.Inventory, 0, 105)
		for i := 0; i < 105; i++ {
			rows = append(rows, &model.Inventory{Base: modelbase.Base{ID: tid*1000 + int64(i) + 1}, WarehouseID: tid*100 + 1, LocationID: 1, SKUID: tid*100 + 1, BatchNo: fmt.Sprintf("%03d", i), StockQuantity: i, AvailableQty: i, StockInTime: time.Now()})
		}
		if err := db.WithContext(tenant.WithExactTenant(ctx, tid)).CreateInBatches(rows, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
	file, err := s.ExportInventory(ctx, &dto.InventoryExportQuery{WarehouseID: 1101, SKUKeyword: "%_"})
	if err != nil {
		t.Fatal(err)
	}
	rows := readExport(t, file)
	if len(rows) != 106 || rows[1][9] != "00123" || rows[1][0] != "11105" {
		t.Fatalf("rows=%d first=%v", len(rows), rows[1])
	}
	// Literal wildcard matching must not broaden to an unrelated SKU query.
	file, err = s.ExportInventory(ctx, &dto.InventoryExportQuery{SKUKeyword: "absent"})
	if err != nil || len(readExport(t, file)) != 1 {
		t.Fatalf("keyword: %v", err)
	}
	file, err = s.ExportInventory(ctx, &dto.InventoryExportQuery{InStockOnly: true})
	if err != nil || len(readExport(t, file)) != 105 {
		t.Fatalf("in stock: %v", err)
	}
	file, err = s.ExportInventory(tenant.WithTenant(context.Background(), 0), &dto.InventoryExportQuery{})
	if err != nil || len(readExport(t, file)) != 106 || readExport(t, file)[1][1] != "1" {
		t.Fatalf("platform scope: %v", err)
	}
	file, err = s.ExportInventory(ctx, &dto.InventoryExportQuery{WarehouseID: 2201})
	if err != nil || len(readExport(t, file)) != 1 {
		t.Fatalf("cross tenant: %v", err)
	}
	// Even a mismatched foreign reference must not expose the other tenant's catalog.
	if err := db.WithContext(ctx).Model(&model.Inventory{}).Where("id=?", 11105).Update("sku_id", 2201).Error; err != nil {
		t.Fatal(err)
	}
	file, err = s.ExportInventory(ctx, &dto.InventoryExportQuery{})
	if err != nil || readExport(t, file)[1][8] != "" {
		t.Fatalf("foreign catalog leak: %v", err)
	}
	if err := db.WithContext(ctx).Delete(&model.Inventory{}, 11104).Error; err != nil {
		t.Fatal(err)
	}
	file, err = s.ExportInventory(ctx, &dto.InventoryExportQuery{})
	if err != nil || len(readExport(t, file)) != 105 {
		t.Fatalf("soft deletion: %v", err)
	}
}

func TestTransactionExportPreservesLedgerAndFilter(t *testing.T) {
	s, db, ctx := alertFixture(t)
	for _, tid := range []int64{0, 11, 22} {
		for i := int64(1); i <= 2; i++ {
			row := &model.InventoryTrans{ID: tid*100 + i, InventoryID: 99, TransType: model.TransAllocate, OrderNo: "RK%_", Operator: "=text", QuantityChange: 0, BeforeQuantity: 5, AfterQuantity: 5, AvailableBefore: 5, AvailableAfter: 2, CreatedAt: time.Now()}
			if err := db.WithContext(tenant.WithExactTenant(ctx, tid)).Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	file, err := s.ExportTransactions(ctx, &dto.TransExportQuery{InventoryID: 99, OrderNo: "%_", TransType: "ALLOCATE"})
	if err != nil {
		t.Fatal(err)
	}
	rows := readExport(t, file)
	if len(rows) != 3 || rows[1][0] != "1102" || rows[1][11] != "5" || rows[1][12] != "5" || rows[1][13] != "5" || rows[1][14] != "2" || rows[1][17] != "=text" {
		t.Fatalf("ledger %v", rows)
	}
	file, err = s.ExportTransactions(ctx, &dto.TransExportQuery{TransType: "SHIP"})
	if err != nil || len(readExport(t, file)) != 1 {
		t.Fatalf("filter: %v", err)
	}
}

func TestInventoryExportRejectsOverLimitAndCancelled(t *testing.T) {
	s, db, ctx := alertFixture(t)
	rows := make([]*model.Inventory, 10001)
	for i := range rows {
		rows[i] = &model.Inventory{Base: modelbase.Base{ID: int64(i + 1)}, WarehouseID: 1101, LocationID: 1, SKUID: 1101, BatchNo: fmt.Sprint(i), StockInTime: time.Now()}
	}
	if err := db.WithContext(ctx).CreateInBatches(rows, 500).Error; err != nil {
		t.Fatal(err)
	}
	file, err := s.ExportInventory(ctx, &dto.InventoryExportQuery{})
	if !errors.Is(err, xlsxexport.TooManyRows) || file != nil {
		t.Fatalf("partial success: %v %v", file, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = s.ExportInventory(cancelled, &dto.InventoryExportQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Database failure is not turned into a successful empty workbook.
	injected := errors.New("injected read failure")
	if err := db.Callback().Row().Before("gorm:row").Register("export_query_error", func(tx *gorm.DB) { _ = tx.AddError(injected) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Row().Remove("export_query_error")
	file, err = s.ExportInventory(ctx, &dto.InventoryExportQuery{})
	if file != nil || !errors.Is(err, injected) {
		t.Fatalf("read failure swallowed: %v %v", file, err)
	}
}
