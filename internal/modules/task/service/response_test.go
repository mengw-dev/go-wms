package service

import (
	"encoding/json"
	"testing"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/modules/task/model"
)

func TestTaskResponseKeepsPublicFieldsAndHidesInternalVersion(t *testing.T) {
	task := &model.Task{
		Base:         sysmodel.Base{ID: 42},
		Versioned:    sysmodel.Versioned{Version: 7},
		TenantID:     99,
		TaskNo:       "PK202609230001",
		TaskType:     model.TaskPick,
		Status:       model.TaskCreated,
		OrderID:      88,
		OrderNo:      "CK-88",
		DetailID:     77,
		AllocationID: 66,
		SKUID:        55,
		WarehouseID:  44,
		LocationID:   33,
		LocationCode: "A-01",
		BatchNo:      "B001",
		TargetQty:    10,
		DoneQty:      3,
		Operator:     "picker",
	}

	resp := taskResponse(task)
	if resp.ID != task.ID || resp.TaskNo != task.TaskNo || resp.DoneQty != task.DoneQty {
		t.Fatalf("task response fields changed: %+v", resp)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal task response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal task response: %v", err)
	}
	if _, exists := fields["version"]; exists {
		t.Fatalf("task response exposes internal version: %s", raw)
	}
	if fields["id"] != "42" || fields["order_id"] != "88" {
		t.Fatalf("task response ID contract changed: %s", raw)
	}
}
