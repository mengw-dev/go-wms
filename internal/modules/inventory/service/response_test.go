package service

import (
	"encoding/json"
	"testing"
	"time"

	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	sysmodel "gowms/internal/modules/system/model"
)

func TestInventoryResponsesKeepQuantityContractAndHideVersion(t *testing.T) {
	stockIn := time.Date(2026, 9, 23, 1, 2, 3, 0, time.UTC)
	inventory := &invmodel.Inventory{
		Base:          sysmodel.Base{ID: 42},
		Versioned:     sysmodel.Versioned{Version: 7},
		TenantID:      11,
		WarehouseID:   21,
		LocationID:    31,
		SKUID:         41,
		BatchNo:       "B001",
		StockQuantity: 10, AvailableQty: 6, AllocatedQty: 4,
		StockInTime: stockIn, LocationCode: "A-01",
	}
	resp := inventoryResponses([]*invmodel.Inventory{inventory})[0]
	if resp.StockQuantity != 10 || resp.AvailableQty != 6 || resp.AllocatedQty != 4 {
		t.Fatalf("quantity contract changed: %+v", resp)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal inventory response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal inventory response: %v", err)
	}
	if _, exists := fields["version"]; exists {
		t.Fatalf("inventory response exposes version: %s", raw)
	}
	if fields["id"] != "42" || fields["warehouse_id"] != "21" {
		t.Fatalf("inventory response ID contract changed: %s", raw)
	}
}

func TestInventoryTransResponseKeepsTraceFields(t *testing.T) {
	trans := &invmodel.InventoryTrans{
		ID: 9, TenantID: 11, InventoryID: 42, TransType: invmodel.TransAllocate,
		QuantityChange: 0, BeforeQuantity: 10, AfterQuantity: 10,
		AvailableBefore: 10, AvailableAfter: 4, OrderNo: "CK-1", TaskNo: "PK-1", Operator: "tester",
	}
	resp := inventoryTransResponses([]*invmodel.InventoryTrans{trans})[0]
	if resp.TransType != string(invmodel.TransAllocate) || resp.AvailableBefore != 10 || resp.AvailableAfter != 4 {
		t.Fatalf("inventory transaction response changed: %+v", resp)
	}
}

func TestInventorySummaryResponseKeepsQuantityContract(t *testing.T) {
	rows := []*repository.SummaryRow{{
		SKUID: 42, SKUCode: "SKU01", SKUName: "Item", Unit: "box",
		StockQuantity: 10, AvailableQty: 6, AllocatedQty: 4,
	}}
	resp := inventorySummaryResponses(rows)[0]
	if resp.SKUID != 42 || resp.StockQuantity != 10 || resp.AvailableQty != 6 || resp.AllocatedQty != 4 {
		t.Fatalf("summary response changed: %+v", resp)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal summary response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal summary response: %v", err)
	}
	if fields["sku_id"] != "42" || fields["stock_quantity"] != float64(10) {
		t.Fatalf("summary response contract changed: %s", raw)
	}
}
