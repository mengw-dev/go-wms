package service

import (
	"context"
	"testing"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
)

func TestPlatformImportResolvesOnlyOwnCatalog(t *testing.T) {
	s, db, _ := importFixture(t) // tenant 11 owns WH/SKU with the lower IDs.
	platform := context.Background()
	exact := tenant.WithExactTenant(platform, 0)
	for _, row := range []any{
		&basicmodel.Warehouse{Base: modelbase.Base{ID: 2}, Code: "WH", Name: "Platform", Status: 1},
		&basicmodel.SKU{Base: modelbase.Base{ID: 2}, Code: "SKU", Barcode: "BAR", Name: "Platform", Status: 1},
	} {
		if err := db.WithContext(exact).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	resp, err := s.Import(platform, "platform.xlsx", importWorkbook(t, [][]any{
		{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", "2"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetImport(exact, resp.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	// Dispatcher remains global; the claimed task determines the exact scope.
	if err := s.processImport(platform, task); err != nil {
		t.Fatal(err)
	}
	finished, err := s.GetImport(exact, resp.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != model.ImportCompleted || finished.SuccessRows != 1 || finished.FailRows != 0 {
		t.Fatalf("platform import = %+v", finished)
	}
	var orders []model.ReceiptOrder
	if err := db.Find(&orders).Error; err != nil {
		t.Fatal(err)
	}
	if len(orders) != 1 || orders[0].TenantID != 0 || orders[0].WarehouseID != 2 {
		t.Fatalf("wrong catalog/owner: %+v", orders)
	}
	var details []model.ReceiptOrderDetail
	if err := db.Find(&details).Error; err != nil {
		t.Fatal(err)
	}
	if len(details) != 1 || details[0].TenantID != 0 || details[0].SKUID != 2 {
		t.Fatalf("wrong SKU/owner: %+v", details)
	}
}
