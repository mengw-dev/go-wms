package service

import (
	"encoding/json"
	"testing"
	"time"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/modules/task/model"
)

func TestTaskResponseKeepsPublicFieldsAndHidesInternalVersion(t *testing.T) {
	task := &model.Task{
		Base:         sysmodel.Base{ID: 42},
		Versioned:    sysmodel.Versioned{Version: 7},
		TenantID:     99,
		TaskNo:       "PK202609230001",
		TaskType:     model.TaskPick,
		Status:       model.TaskCreated,
		OrderID:      88,
		OrderNo:      "CK-88",
		DetailID:     77,
		AllocationID: 66,
		SKUID:        55,
		WarehouseID:  44,
		LocationID:   33,
		LocationCode: "A-01",
		BatchNo:      "B001",
		TargetQty:    10,
		DoneQty:      3,
		Operator:     "picker",
	}

	resp := taskResponse(task)
	if resp.ID != task.ID || resp.TaskNo != task.TaskNo || resp.DoneQty != task.DoneQty {
		t.Fatalf("task response fields changed: %+v", resp)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal task response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal task response: %v", err)
	}
	if _, exists := fields["version"]; exists {
		t.Fatalf("task response exposes internal version: %s", raw)
	}
	if fields["id"] != "42" || fields["order_id"] != "88" {
		t.Fatalf("task response ID contract changed: %s", raw)
	}
}

// TestTaskResponseLeaseFields 未领取任务不输出租约字段；已领取任务原样返回领取人与到期时间。
func TestTaskResponseLeaseFields(t *testing.T) {
	withoutLease := taskResponse(&model.Task{Base: sysmodel.Base{ID: 1}})
	raw, err := json.Marshal(withoutLease)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["claimed_by"]; exists {
		t.Fatalf("unclaimed task exposes claimed_by: %s", raw)
	}
	if _, exists := fields["lease_expire_at"]; exists {
		t.Fatalf("unclaimed task exposes lease_expire_at: %s", raw)
	}

	expireAt := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	withLease := taskResponse(&model.Task{
		Base: sysmodel.Base{ID: 2}, ClaimedBy: "picker-a", LeaseExpireAt: &expireAt,
	})
	if withLease.ClaimedBy != "picker-a" || !withLease.LeaseExpireAt.Equal(expireAt) {
		t.Fatalf("lease fields not mapped: %+v", withLease)
	}
	raw, err = json.Marshal(withLease)
	if err != nil {
		t.Fatalf("marshal leased task: %v", err)
	}
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["claimed_by"] != "picker-a" {
		t.Fatalf("claimed_by contract changed: %s", raw)
	}
}
