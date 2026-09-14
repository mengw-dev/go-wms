package dto

import (
	"encoding/json"
	"testing"
)

func TestCreateOrderReqAcceptsStringIDs(t *testing.T) {
	payload := []byte(`{"warehouse_id":"9007199254740993","details":[{"sku_id":"9007199254740995","expected_qty":2}]}`)
	var req CreateOrderReq
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if req.WarehouseID != 9007199254740993 || req.Details[0].SKUID != 9007199254740995 {
		t.Fatalf("unexpected ids: %+v", req)
	}
}
