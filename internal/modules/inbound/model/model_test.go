package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestCanTransit 入库单状态机：合法流转全通过，非法/跨状态/终态流转全拒绝。
func TestCanTransit(t *testing.T) {
	legal := map[OrderStatus][]OrderStatus{
		OrderDraft:     {OrderSubmitted, OrderCancelled},
		OrderSubmitted: {OrderApproved, OrderCancelled},
		OrderApproved:  {OrderReceiving, OrderPutaway, OrderCompleted, OrderCancelled},
		OrderReceiving: {OrderPutaway, OrderCompleted},
		OrderPutaway:   {OrderCompleted},
	}
	for from, tos := range legal {
		for _, to := range tos {
			if !CanTransit(from, to) {
				t.Errorf("expect legal: %s -> %s", from, to)
			}
		}
	}

	illegal := []struct {
		from, to OrderStatus
	}{
		// 跨状态跳转
		{OrderDraft, OrderApproved},
		{OrderDraft, OrderCompleted},
		{OrderSubmitted, OrderReceiving},
		{OrderSubmitted, OrderCompleted},
		{OrderApproved, OrderSubmitted},
		{OrderReceiving, OrderApproved},
		// 终态无后继
		{OrderCompleted, OrderDraft},
		{OrderCompleted, OrderPutaway},
		{OrderCancelled, OrderSubmitted},
		{OrderCancelled, OrderCompleted},
		// 回退
		{OrderReceiving, OrderDraft},
		{OrderPutaway, OrderReceiving},
	}
	for _, c := range illegal {
		if CanTransit(c.from, c.to) {
			t.Errorf("expect illegal: %s -> %s", c.from, c.to)
		}
	}
}

func TestImportTaskJSONDoesNotExposeExecutionSecrets(t *testing.T) {
	task := ImportTask{
		RunToken: strings.Repeat("r", 36),
		FileName: "orders.xlsx",
		FilePath: "/tmp/private/orders.xlsx",
	}

	got, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal import task: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("unmarshal import task: %v", err)
	}
	for _, field := range []string{"run_token", "file_path"} {
		if _, exists := fields[field]; exists {
			t.Fatalf("internal field %q must not be exposed, got %s", field, got)
		}
	}
	if fields["file_name"] != "orders.xlsx" {
		t.Fatalf("public file name changed, got %s", got)
	}
}
