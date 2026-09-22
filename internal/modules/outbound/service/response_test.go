package service

import (
	"encoding/json"
	"strings"
	"testing"

	"gowms/internal/modules/outbound/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
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
func TestOrderDetailResponsesHideInternalVersions(t *testing.T) {
	detail := &model.ShipmentOrderDetail{
		Base:        sysmodel.Base{ID: 21},
		TenantID:    99,
		OrderID:     42,
		SKUID:       7,
		SKUCode:     "SKU-7",
		SKUName:     "货品 7",
		ExpectedQty: 10,
		PickedQty:   3,
	}
	allocation := &model.Allocation{
		Base:         sysmodel.Base{ID: 31},
		Versioned:    sysmodel.Versioned{Version: 2},
		TenantID:     99,
		OrderID:      42,
		DetailID:     21,
		InventoryID:  11,
		SKUID:        7,
		LocationID:   5,
		LocationCode: "A-01",
		BatchNo:      "B001",
		AllocatedQty: 8,
		PickedQty:    3,
		Status:       model.AllocAllocated,
	}
	task := &taskmodel.Task{
		Base:         sysmodel.Base{ID: 51},
		Versioned:    sysmodel.Versioned{Version: 4},
		TenantID:     99,
		TaskNo:       "PK-51",
		TaskType:     taskmodel.TaskPick,
		Status:       taskmodel.TaskInProgress,
		OrderID:      42,
		DetailID:     21,
		AllocationID: 31,
		SKUID:        7,
		WarehouseID:  11,
		TargetQty:    8,
		DoneQty:      3,
	}

	responses := map[string]any{
		"detail":     orderDetailRowResponses([]*model.ShipmentOrderDetail{detail})[0],
		"allocation": allocationResponses([]*model.Allocation{allocation})[0],
		"task":       orderTaskResponses([]*taskmodel.Task{task})[0],
	}
	for name, response := range responses {
		raw, err := json.Marshal(response)
		if err != nil {
			t.Fatalf("marshal %s response: %v", name, err)
		}
		if strings.Contains(string(raw), `"version"`) {
			t.Fatalf("%s response exposes internal version: %s", name, raw)
		}
	}
}
