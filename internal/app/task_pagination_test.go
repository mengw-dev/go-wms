package app

import (
	"context"
	"fmt"
	"testing"

	indto "gowms/internal/modules/inbound/dto"
	outdto "gowms/internal/modules/outbound/dto"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/tenant"
)

func TestOrderDetailsLoadTasksBeyondFirstPage(t *testing.T) {
	s := newOrderReferenceStack(t)
	in, err := s.in.Create(s.ctx, &indto.CreateOrderReq{WarehouseID: s.warehouseID, Details: []indto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 2}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "PAGED", Details: []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 2}}}, "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, orderID := range []int64{in.ID, out.ID} {
		rows := make([]*taskmodel.Task, 0, 252)
		for i := range 251 {
			rows = append(rows, &taskmodel.Task{TenantID: 81, TaskNo: fmt.Sprintf("PAGE-%d-%d", orderID, i), TaskType: taskmodel.TaskPutaway, OrderID: orderID, TargetQty: 1})
		}
		rows = append(rows, &taskmodel.Task{TenantID: 82, TaskNo: fmt.Sprintf("FOREIGN-%d", orderID), TaskType: taskmodel.TaskPutaway, OrderID: orderID, TargetQty: 1})
		if err := s.db.CreateInBatches(rows, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
	inDetail, err := s.in.Get(s.ctx, in.ID)
	if err != nil || len(inDetail.Tasks) != 251 {
		t.Fatalf("inbound tasks truncated: %v %v", inDetail, err)
	}
	outDetail, err := s.out.Get(s.ctx, out.ID)
	if err != nil || len(outDetail.Tasks) != 251 {
		t.Fatalf("outbound tasks truncated: %v %v", outDetail, err)
	}
	for _, tasks := range [][]*taskmodel.Task{inDetail.Tasks, outDetail.Tasks} {
		seen := make(map[int64]bool)
		for _, task := range tasks {
			if task.TenantID != 81 || seen[task.ID] {
				t.Fatal("tenant leak or duplicate task")
			}
			seen[task.ID] = true
		}
	}
	canceled, cancel := context.WithCancel(tenant.WithTenant(context.Background(), 81))
	cancel()
	if _, err := s.in.Get(canceled, in.ID); err == nil {
		t.Fatal("cancellation ignored")
	}
}
