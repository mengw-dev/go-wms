package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func TestGetOrderPreservesTenantAndCancellation(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.ReceiptOrder{}, &model.ReceiptOrderDetail{}, &taskmodel.Task{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithTenant(context.Background(), 101)
	order := &model.ReceiptOrder{OrderNo: "TEST-CONTEXT", WarehouseID: 1}
	if err := db.WithContext(ctx).Create(order).Error; err != nil {
		t.Fatal(err)
	}
	// 主单 ID 即使匹配，详情查询也必须保持请求租户，不能靠调用顺序代替隔离。
	for _, tenantID := range []int64{101, 202} {
		detail := &model.ReceiptOrderDetail{TenantID: tenantID, OrderID: order.ID, SKUID: 1, ExpectedQty: 1}
		if err := db.Create(detail).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := &Service{repo: repository.New(), tm: tx.New(db), taskAPI: taskservice.New(taskrepo.New(), db)}
	got, err := s.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Details) != 1 || got.Details[0].TenantID != 101 {
		t.Fatalf("detail tenant scope lost: %+v", got.Details)
	}
	if _, err := s.Get(tenant.WithTenant(context.Background(), 202), order.ID); !errors.Is(err, errcode.OrderNotFound) {
		t.Fatalf("other tenant: got %v, want order not found", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Get(canceled, order.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation became a business error: %v", err)
	}
}
