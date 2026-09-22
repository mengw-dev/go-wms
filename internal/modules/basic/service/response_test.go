package service

import (
	"encoding/json"
	"testing"

	"gowms/internal/modules/basic/model"
	sysmodel "gowms/internal/modules/system/model"
)

func TestBasicResponsesKeepPublicFieldsAndStringIDs(t *testing.T) {
	warehouse := &model.Warehouse{Base: sysmodel.Base{ID: 11}, TenantID: 99, Code: "WH01", Name: "Main", Status: 1}
	location := &model.Location{Base: sysmodel.Base{ID: 22}, TenantID: 99, WarehouseID: 11, Code: "A-01", Zone: "A", Status: 1}
	sku := &model.SKU{Base: sysmodel.Base{ID: 33}, TenantID: 99, Code: "SKU01", Barcode: "69001", Name: "Item", Status: 1}

	responses := map[string]any{
		"warehouse": warehouseResponse(warehouse),
		"location":  locationResponse(location),
		"sku":       skuResponse(sku),
	}
	for name, value := range responses {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s response: %v", name, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("unmarshal %s response: %v", name, err)
		}
		if fields["id"] == nil || fields["tenant_id"] == nil {
			t.Fatalf("%s response lost ID fields: %s", name, raw)
		}
		if fields["id"].(string) != map[string]string{"warehouse": "11", "location": "22", "sku": "33"}[name] {
			t.Fatalf("%s response changed ID contract: %s", name, raw)
		}
	}
}
