package service

import (
	"encoding/json"
	"testing"

	"gowms/internal/modules/outbound/model"
	sysmodel "gowms/internal/modules/system/model"
)

func TestOrderResponseKeepsPublicFieldsAndHidesInternalVersion(t *testing.T) {
	order := &model.ShipmentOrder{
		Base:         sysmodel.Base{ID: 42},
		Versioned:    sysmodel.Versioned{Version: 7},
		TenantID:     99,
		OrderNo:      "CK202609230001",
		BizOrderNo:   "BIZ-1",
		WarehouseID:  11,
		Status:       model.OrderPicking,
		Remark:       "test order",
		ExpectedQty:  10,
		AllocatedQty: 8,
		PickedQty:    3,
		CreatedBy:    "operator",
	}
	resp := orderResponse(order)
	if resp.ID != order.ID || resp.BizOrderNo != order.BizOrderNo || resp.PickedQty != order.PickedQty {
		t.Fatalf("order response fields changed: %+v", resp)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal order response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal order response: %v", err)
	}
	if _, exists := fields["version"]; exists {
		t.Fatalf("order response exposes internal version: %s", raw)
	}
	if fields["id"] != "42" || fields["warehouse_id"] != "11" {
		t.Fatalf("order response ID contract changed: %s", raw)
	}
}
