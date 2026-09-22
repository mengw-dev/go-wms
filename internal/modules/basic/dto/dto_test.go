package dto

import (
	"encoding/json"
	"testing"
)

func TestLocationBatchRespJSONContract(t *testing.T) {
	raw, err := json.Marshal(LocationBatchResp{Created: 3})
	if err != nil {
		t.Fatalf("marshal location batch response: %v", err)
	}
	if string(raw) != `{"created":3}` {
		t.Fatalf("unexpected location batch response: %s", raw)
	}
}
