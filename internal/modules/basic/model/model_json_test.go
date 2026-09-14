package model

import (
	"encoding/json"
	"strings"
	"testing"

	sysmodel "gowms/internal/modules/system/model"
)

func TestExternalIDsAreJSONStrings(t *testing.T) {
	warehouse := Warehouse{Base: sysmodel.Base{ID: 9007199254740993}, Code: "WH-1"}
	encoded, err := json.Marshal(warehouse)
	if err != nil {
		t.Fatalf("marshal warehouse: %v", err)
	}
	if !strings.Contains(string(encoded), `"id":"9007199254740993"`) {
		t.Fatalf("warehouse id is not a JSON string: %s", encoded)
	}

	location := Location{Base: sysmodel.Base{ID: 2}, WarehouseID: 9007199254740993}
	encoded, err = json.Marshal(location)
	if err != nil {
		t.Fatalf("marshal location: %v", err)
	}
	if !strings.Contains(string(encoded), `"warehouse_id":"9007199254740993"`) {
		t.Fatalf("location warehouse_id is not a JSON string: %s", encoded)
	}
}
