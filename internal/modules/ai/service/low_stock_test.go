package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	basicmodel "gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/tenant"
)

func TestLowStockAggregatesAllBatchesBeforeFiltering(t *testing.T) {
	s, db := newTestService(t)
	for i, quantities := range [][]int{{5, 100}, {30, 30}, {10, 20}, {0}, {1}} {
		tid := int64(11)
		if i == 4 {
			tid = 22
		}
		sku := basicmodel.SKU{TenantID: tid, Code: fmt.Sprintf("SKU-%d", i), Barcode: fmt.Sprintf("BC-%d", i), Name: "test", Status: 1}
		if err := db.Create(&sku).Error; err != nil {
			t.Fatal(err)
		}
		for j, qty := range quantities {
			row := invmodel.Inventory{TenantID: tid, WarehouseID: 1, LocationID: 1, SKUID: sku.ID, BatchNo: fmt.Sprintf("B-%d", j), StockQuantity: qty, AvailableQty: qty, StockInTime: time.Now()}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
		// 已删除行不参与总量，避免将真正低库存 SKU 排除。
		if i == 2 {
			row := invmodel.Inventory{TenantID: tid, WarehouseID: 1, LocationID: 1, SKUID: sku.ID, BatchNo: "deleted", StockQuantity: 100, AvailableQty: 100, StockInTime: time.Now()}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Delete(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx := tenant.WithExactTenant(context.Background(), 11)
	rows, err := s.repo.LowAvailableSKU(ctx, 50, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].SKUCode != "SKU-3" || rows[0].AvailableQty != 0 || rows[1].SKUCode != "SKU-2" || rows[1].AvailableQty != 30 {
		t.Fatalf("incorrect aggregation/filter/order: %+v", rows)
	}
	rows, err = s.repo.LowAvailableSKU(ctx, 50, 1)
	if err != nil || len(rows) != 1 || rows[0].AvailableQty != 0 {
		t.Fatalf("limit: %+v %v", rows, err)
	}
}
