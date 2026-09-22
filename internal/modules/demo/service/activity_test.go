package service

import (
	"encoding/json"
	"testing"
	"time"

	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
)

func TestActivitySnapshotKeepsBusinessRecordsAndHidesInternalVersion(t *testing.T) {
	createdAt := time.Date(2026, 9, 23, 3, 4, 5, 0, time.UTC)
	importTaskID := "import-1"

	snapshot := activitySnapshot(
		[]*sysmodel.SysOperLog{{
			ID: 1, TenantID: 7, UserID: 2, Username: "demo1", Path: "/inbound/orders",
			Method: "POST", IP: "127.0.0.1", CostMs: 12, Status: 200, Result: `{"code":0}`,
			CreatedAt: createdAt,
		}},
		[]*inboundmodel.ReceiptOrder{{
			Base:      sysmodel.Base{ID: 11, CreatedAt: createdAt},
			Versioned: sysmodel.Versioned{Version: 3},
			TenantID:  7, OrderNo: "RK-1", WarehouseID: 21, Status: inboundmodel.OrderCompleted,
			Source: inboundmodel.SourceImport, ExpectedQty: 10, ReceivedQty: 10,
			ImportTaskID: &importTaskID, ImportRow: 1, CreatedBy: "demo1",
		}},
		[]*outboundmodel.ShipmentOrder{{
			Base:      sysmodel.Base{ID: 12, CreatedAt: createdAt},
			Versioned: sysmodel.Versioned{Version: 4},
			TenantID:  7, OrderNo: "CK-1", BizOrderNo: "BIZ-1", WarehouseID: 21,
			Status: outboundmodel.OrderPicking, ExpectedQty: 5, AllocatedQty: 5, PickedQty: 2,
			CreatedBy: "demo1",
		}},
		[]*stocktakemodel.StocktakeOrder{{
			Base:      sysmodel.Base{ID: 13, CreatedAt: createdAt},
			Versioned: sysmodel.Versioned{Version: 5},
			TenantID:  7, OrderNo: "PD-1", WarehouseID: 21, LocationID: 31, LocationCode: "A-01",
			Status: stocktakemodel.OrderDraft, CreatedBy: "demo1",
		}},
		[]*taskmodel.Task{{
			Base:      sysmodel.Base{ID: 14, CreatedAt: createdAt},
			Versioned: sysmodel.Versioned{Version: 6},
			TenantID:  7, TaskNo: "PK-1", TaskType: taskmodel.TaskPick,
			Status: taskmodel.TaskInProgress, OrderID: 12, OrderNo: "CK-1", SKUID: 41,
			WarehouseID: 21, TargetQty: 5, DoneQty: 2, Operator: "demo1",
		}},
		[]*invmodel.InventoryTrans{{
			ID: 15, TenantID: 7, InventoryID: 51, TransType: invmodel.TransAllocate,
			BeforeQuantity: 10, AfterQuantity: 10, AvailableBefore: 10, AvailableAfter: 5,
			OrderNo: "CK-1", TaskNo: "PK-1", Operator: "demo1", CreatedAt: createdAt,
		}},
	)

	fields := marshalActivityFields(t, snapshot)
	if fields["operations"].([]any)[0].(map[string]any)["id"] != "1" {
		t.Fatalf("operation ID contract changed: %+v", fields["operations"])
	}
	inbound := firstActivityObject(t, fields, "inbound_orders")
	if inbound["id"] != "11" || inbound["tenant_id"] != "7" || inbound["status"] != "COMPLETED" {
		t.Fatalf("inbound activity contract changed: %+v", inbound)
	}
	outbound := firstActivityObject(t, fields, "outbound_orders")
	if outbound["biz_order_no"] != "BIZ-1" || outbound["picked_qty"] != float64(2) {
		t.Fatalf("outbound activity contract changed: %+v", outbound)
	}
	stocktake := firstActivityObject(t, fields, "stocktake_orders")
	if stocktake["location_code"] != "A-01" || stocktake["status"] != "DRAFT" {
		t.Fatalf("stocktake activity contract changed: %+v", stocktake)
	}
	task := firstActivityObject(t, fields, "tasks")
	if task["task_type"] != "PICK" || task["done_qty"] != float64(2) {
		t.Fatalf("task activity contract changed: %+v", task)
	}
	trans := firstActivityObject(t, fields, "inventory_trans")
	if trans["trans_type"] != "ALLOCATE" || trans["available_after"] != float64(5) {
		t.Fatalf("inventory transaction activity contract changed: %+v", trans)
	}

	for _, key := range []string{"inbound_orders", "outbound_orders", "stocktake_orders", "tasks"} {
		item := firstActivityObject(t, fields, key)
		if _, exists := item["version"]; exists {
			t.Fatalf("%s exposes internal version: %+v", key, item)
		}
	}
}

func marshalActivityFields(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal activity response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal activity response: %v", err)
	}
	return fields
}

func firstActivityObject(t *testing.T, fields map[string]any, key string) map[string]any {
	t.Helper()
	items, ok := fields[key].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("%s response shape changed: %+v", key, fields[key])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("%s item shape changed: %+v", key, items[0])
	}
	return item
}
