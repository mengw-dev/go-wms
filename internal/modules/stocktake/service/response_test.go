package service

import (
	"encoding/json"
	"testing"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
)

func TestStocktakeResponsesKeepPublicFieldsAndHideVersion(t *testing.T) {
	actual := 8
	order := &model.StocktakeOrder{
		Base: sysmodel.Base{ID: 42}, Versioned: sysmodel.Versioned{Version: 7},
		TenantID: 11, OrderNo: "PD-1", WarehouseID: 21, LocationID: 31,
		LocationCode: "A-01", Status: model.OrderDraft, Remark: "count", CreatedBy: "tester",
	}
	detail := &model.StocktakeDetail{
		Base: sysmodel.Base{ID: 9}, TenantID: 11, OrderID: 42, InventoryID: 55,
		SKUID: 66, SKUCode: "SKU01", SKUName: "Item", LocationID: 31, LocationCode: "A-01",
		BatchNo: "B001", BookQty: 10, ActualQty: &actual, DiffQty: -2, Adjusted: true,
	}
	resp := &dto.OrderDetailResp{Order: orderResponse(order), Details: detailResponses([]*model.StocktakeDetail{detail})}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal stocktake response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal stocktake response: %v", err)
	}
	if _, exists := fields["version"]; exists {
		t.Fatalf("stocktake response exposes version: %s", raw)
	}
	if resp.Order.ID != 42 || resp.Details[0].ActualQty == nil || *resp.Details[0].ActualQty != 8 {
		t.Fatalf("stocktake response changed counts: %+v", resp)
	}
}
