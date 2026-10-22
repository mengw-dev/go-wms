package service

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/xlsxexport"
)

func exportData(t *testing.T, file *xlsxexport.File) [][]string {
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

func TestStocktakeExportsSnapshotDifferenceAndApprovedAdjustment(t *testing.T) {
	s, db, ctx, _ := stocktakeFixture(t)
	for _, tid := range []int64{0, 11, 22} {
		scoped := tenant.WithExactTenant(ctx, tid)
		for i, status := range []model.OrderStatus{model.OrderDraft, model.OrderCompleted, model.OrderCancelled} {
			orderID := tid*100 + int64(i) + 1
			order := &model.StocktakeOrder{Base: modelbase.Base{ID: orderID}, WarehouseID: 1, OrderNo: []string{"PD%_A", "PD-COMPLETE", "PD-CANCELLED"}[i], Status: status}
			if err := db.WithContext(scoped).Create(order).Error; err != nil {
				t.Fatal(err)
			}
			actual := 0
			if i == 1 {
				actual = 10
			}
			rows := []*model.StocktakeDetail{{Base: modelbase.Base{ID: orderID * 10}, OrderID: orderID, InventoryID: 1, SKUID: 1, SKUCode: "000123", SKUName: "=name", BookQty: 10, ActualQty: &actual}}
			if i == 0 {
				rows = append(rows, &model.StocktakeDetail{Base: modelbase.Base{ID: orderID*10 + 1}, OrderID: orderID, InventoryID: 1, SKUID: 1, BookQty: 10})
			}
			if i == 1 {
				rows[0].DiffQty = -2
				rows[0].Adjusted = true
			}
			if err := db.WithContext(scoped).Create(rows).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	file, err := s.ExportDetails(ctx, &dto.ExportQuery{OrderID: 1101})
	if err != nil {
		t.Fatal(err)
	}
	rows := exportData(t, file)
	if len(rows) != 3 || rows[1][6] != "000123" || rows[1][11] != "0" || rows[1][12] != "-10" || rows[1][13] != "" || rows[2][11] != "" || rows[2][12] != "" {
		t.Fatalf("null/zero: %v", rows)
	}
	file, err = s.ExportDetails(ctx, &dto.ExportQuery{DifferencesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	rows = exportData(t, file)
	if len(rows) != 4 || rows[2][3] != "COMPLETED" || rows[2][12] != "0" || rows[2][13] != "-2" {
		t.Fatalf("two differences confused: %v", rows)
	}
	file, err = s.ExportDetails(ctx, &dto.ExportQuery{Keyword: "%_", DifferencesOnly: true})
	if err != nil || len(exportData(t, file)) != 2 {
		t.Fatalf("literal keyword: %v", err)
	}
	file, err = s.ExportDetails(ctx, &dto.ExportQuery{Status: "COMPLETED", WarehouseID: 1})
	if err != nil || len(exportData(t, file)) != 2 {
		t.Fatalf("status: %v", err)
	}
	file, err = s.ExportDetails(ctx, &dto.ExportQuery{OrderID: 2201})
	if err != nil || len(exportData(t, file)) != 1 {
		t.Fatalf("cross tenant: %v", err)
	}
	file, err = s.ExportDetails(tenant.WithTenant(context.Background(), 0), &dto.ExportQuery{})
	if err != nil || len(exportData(t, file)) != 5 || exportData(t, file)[1][0] != "3" {
		t.Fatalf("platform: %v", err)
	}
	if err := db.WithContext(ctx).Delete(&model.StocktakeOrder{}, 1102).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Delete(&model.StocktakeDetail{}, 11030).Error; err != nil {
		t.Fatal(err)
	}
	file, err = s.ExportDetails(ctx, &dto.ExportQuery{})
	if err != nil || len(exportData(t, file)) != 3 {
		t.Fatalf("deleted details/master: %v", err)
	}
}
