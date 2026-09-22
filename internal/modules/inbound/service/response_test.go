package service

import (
	"encoding/json"
	"strings"
	"testing"

	"gowms/internal/modules/inbound/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
)

func TestImportTaskResponseKeepsPublicFieldsAndHidesInternalPaths(t *testing.T) {
	task := &model.ImportTask{
		TaskID: "IMP-1", Status: model.ImportFailed, FileName: "orders.xlsx",
		FilePath: "/private/orders.xlsx", RunToken: "run-token-value",
		TotalRows: 10, SuccessRows: 8, FailRows: 2, ErrorMsg: "invalid row",
	}
	resp := importTaskResponse(task)
	if resp.TaskID != "IMP-1" || resp.SuccessRows != 8 || resp.FailRows != 2 || resp.ErrorMsg != "invalid row" {
		t.Fatalf("import response fields changed: %+v", resp)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal import response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal import response: %v", err)
	}
	for _, forbidden := range []string{"run_token", "file_path"} {
		if _, exists := fields[forbidden]; exists {
			t.Fatalf("import response exposes %q: %s", forbidden, raw)
		}
	}
}
func TestOrderResponseKeepsPublicFieldsAndHidesInternalVersion(t *testing.T) {
	importTaskID := "IMP-1"
	order := &model.ReceiptOrder{
		Base:         sysmodel.Base{ID: 42},
		Versioned:    sysmodel.Versioned{Version: 7},
		TenantID:     99,
		OrderNo:      "RK202609230001",
		WarehouseID:  11,
		Status:       model.OrderReceiving,
		Source:       model.SourceImport,
		Remark:       "test order",
		ExpectedQty:  10,
		ReceivedQty:  4,
		DefectiveQty: 1,
		ImportTaskID: &importTaskID,
		ImportRow:    3,
		CreatedBy:    "operator",
	}
	resp := orderResponse(order)
	if resp.ID != order.ID || resp.OrderNo != order.OrderNo || resp.ReceivedQty != order.ReceivedQty {
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
	detail := &model.ReceiptOrderDetail{
		Base:         sysmodel.Base{ID: 21},
		TenantID:     99,
		OrderID:      42,
		SKUID:        7,
		SKUCode:      "SKU-7",
		SKUName:      "货品 7",
		ExpectedQty:  10,
		ReceivedQty:  4,
		DefectiveQty: 1,
		BatchNo:      "B001",
	}
	task := &taskmodel.Task{
		Base:        sysmodel.Base{ID: 51},
		Versioned:   sysmodel.Versioned{Version: 4},
		TenantID:    99,
		TaskNo:      "SH-51",
		TaskType:    taskmodel.TaskReceive,
		Status:      taskmodel.TaskInProgress,
		OrderID:     42,
		DetailID:    21,
		SKUID:       7,
		WarehouseID: 11,
		TargetQty:   10,
		DoneQty:     4,
	}
	responses := map[string]any{
		"detail": orderDetailRowResponses([]*model.ReceiptOrderDetail{detail})[0],
		"task":   orderTaskResponses([]*taskmodel.Task{task})[0],
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
