package service

import (
	"context"
	"testing"

	"gowms/internal/modules/task/model"
	"gowms/internal/pkg/tenant"
)

func TestDashboardTaskDrillDownFiltersWarehouseAndTenant(t *testing.T) {
	svc, db, ctx := taskFixture(t)
	for _, row := range []*model.Task{
		{TaskNo: "WH1", WarehouseID: 1, TaskType: "PICK", Status: "CREATED", OrderID: 1, TargetQty: 10},
		{TaskNo: "WH2", WarehouseID: 2, TaskType: "PICK", Status: "CREATED", OrderID: 2, TargetQty: 10},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.WithContext(tenant.WithTenant(context.Background(), 100)).Create(&model.Task{TaskNo: "OTHER", WarehouseID: 2, TaskType: "PICK", Status: "CREATED", OrderID: 2, TargetQty: 10}).Error; err != nil {
		t.Fatal(err)
	}
	rows, total, err := svc.ListResponsesForWarehouse(ctx, 2, 0, "PICK", "CREATED", "", 1, 10)
	if err != nil || total != 1 || len(rows) != 1 || rows[0].TaskNo != "WH2" {
		t.Fatalf("rows=%+v total=%d error=%v", rows, total, err)
	}
	_, total, err = svc.ListResponses(ctx, 0, "PICK", "CREATED", "", 1, 10)
	if err != nil || total != 2 {
		t.Fatalf("unfiltered contract changed: total=%d error=%v", total, err)
	}
}
