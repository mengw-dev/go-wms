package service

import (
	"encoding/json"
	"testing"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
)

func TestStocktakeResponsesKeepPublicFieldsAndHideVersion(t *testing.T) {
	const orderID int64 = 9007199254740993 // 超过 JavaScript 安全整数范围，必须以字符串输出。
	order := &model.StocktakeOrder{
		Base: sysmodel.Base{ID: orderID}, Versioned: sysmodel.Versioned{Version: 7},
		TenantID: 11, OrderNo: "PD-1", WarehouseID: 21, LocationID: 31,
		LocationCode: "A-01", Status: model.OrderDraft, Remark: "count", CreatedBy: "tester",
	}
	detail := &model.StocktakeDetail{
		Base: sysmodel.Base{ID: 9}, TenantID: 11, OrderID: orderID, InventoryID: 55,
		SKUID: 66, SKUCode: "SKU01", SKUName: "Item", LocationID: 31, LocationCode: "A-01",
		BatchNo: "B001", BookQty: 10, DiffQty: -2, Adjusted: true,
	}
	zero, eight := 0, 8
	for _, tc := range []struct {
		name   string
		actual *int
	}{
		{"uncounted", nil}, {"counted_zero", &zero}, {"counted_nonzero", &eight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			detail.ActualQty = tc.actual
			resp := &dto.OrderDetailResp{Order: orderResponse(order), Details: detailResponses([]*model.StocktakeDetail{detail})}
			raw, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal stocktake response: %v", err)
			}
			var fields struct {
				Order   map[string]any   `json:"order"`
				Details []map[string]any `json:"details"`
			}
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields.Order == nil || len(fields.Details) != 1 {
				t.Fatalf("invalid aggregate: %s", raw)
			}
			for name, row := range map[string]map[string]any{"order": fields.Order, "details[0]": fields.Details[0]} {
				for _, hidden := range []string{"version", "deleted_at"} {
					if _, exists := row[hidden]; exists {
						t.Fatalf("%s exposes %s: %s", name, hidden, raw)
					}
				}
			}
			if fields.Order["id"] != "9007199254740993" || fields.Order["warehouse_id"] != "21" || fields.Details[0]["order_id"] != "9007199254740993" {
				t.Fatalf("string ID contract changed: %s", raw)
			}
			actual, exists := fields.Details[0]["actual_qty"]
			if !exists {
				t.Fatalf("actual_qty omitted: %s", raw)
			}
			if tc.actual == nil {
				if actual != nil {
					t.Fatalf("uncounted actual is not null: %s", raw)
				}
			} else if actual != float64(*tc.actual) {
				t.Fatalf("actual quantity changed: %s", raw)
			}
			if fields.Details[0]["book_qty"] != float64(10) || fields.Details[0]["diff_qty"] != float64(-2) || fields.Details[0]["adjusted"] != true {
				t.Fatalf("detail values changed: %s", raw)
			}
		})
	}
}
