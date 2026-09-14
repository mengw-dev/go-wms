package typex

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestInt64ListJSON(t *testing.T) {
	var ids Int64List
	if err := json.Unmarshal([]byte(`["9007199254740993","2"]`), &ids); err != nil {
		t.Fatalf("unmarshal strings: %v", err)
	}
	if !reflect.DeepEqual(ids, Int64List{9007199254740993, 2}) {
		t.Fatalf("unexpected ids: %v", ids)
	}

	encoded, err := json.Marshal(ids)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `["9007199254740993","2"]` {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
}

func TestInt64ListAcceptsLegacyNumbers(t *testing.T) {
	var ids Int64List
	if err := json.Unmarshal([]byte(`[1,2]`), &ids); err != nil {
		t.Fatalf("unmarshal numbers: %v", err)
	}
	if !reflect.DeepEqual(ids, Int64List{1, 2}) {
		t.Fatalf("unexpected ids: %v", ids)
	}
}
