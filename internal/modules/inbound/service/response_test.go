package service

import (
	"encoding/json"
	"testing"

	"gowms/internal/modules/inbound/model"
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
