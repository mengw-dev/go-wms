package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/bootstrap"
	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	indto "gowms/internal/modules/inbound/dto"
	inmodel "gowms/internal/modules/inbound/model"
	inrepo "gowms/internal/modules/inbound/repository"
	inservice "gowms/internal/modules/inbound/service"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	outrepo "gowms/internal/modules/outbound/repository"
	outservice "gowms/internal/modules/outbound/service"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

type orderReferenceStack struct {
	db                 *gorm.DB
	ctx                context.Context
	basic              *basicservice.Service
	in                 *inservice.Service
	out                *outservice.Service
	warehouseID, skuID int64
}

func newOrderReferenceStack(t *testing.T) *orderReferenceStack {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn)
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 81), 15*time.Second)
	t.Cleanup(cancel)
	tm := tx.New(db)
	inv := invservice.New(invrepo.New(), tm)
	basic := basicservice.New(basicrepo.New(), tm, nil, inv, config.LimitsConfig{})
	tasks := taskservice.New(taskrepo.New(), db)
	stack := &orderReferenceStack{db: db, ctx: ctx, basic: basic,
		in:          inservice.New(inrepo.New(), tm, orderno.New(nil), basic, inv, tasks, config.UploadConfig{}, config.LimitsConfig{}),
		out:         outservice.New(outrepo.New(), tm, orderno.New(nil), basic, inv, tasks, config.LimitsConfig{}),
		warehouseID: snowflake.Next(), skuID: snowflake.Next()}
	for _, row := range []any{
		&basicmodel.Warehouse{Base: modelbase.Base{ID: stack.warehouseID}, Code: "ORDER-WH", Name: "test", Status: 1},
		&basicmodel.SKU{Base: modelbase.Base{ID: stack.skuID}, Code: "ORDER-SKU", Barcode: "ORDER-BC", Name: "test", Status: 1},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	return stack
}

func (s *orderReferenceStack) create(kind string) error {
	if kind == "inbound" {
		_, err := s.in.Create(s.ctx, &indto.CreateOrderReq{WarehouseID: s.warehouseID, Details: []indto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 2}}}, "test")
		return err
	}
	_, err := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: fmt.Sprintf("ORDER-%d", s.skuID), Details: []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 2}}}, "test")
	return err
}

func TestInboundEditUsesSingleTransactionConnection(t *testing.T) {
	s := newOrderReferenceStack(t)
	pool, err := s.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	req := indto.CreateOrderReq{WarehouseID: s.warehouseID, Details: []indto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 2}}}
	order, err := s.in.Create(ctx, &req, "test")
	if err != nil {
		t.Fatal(err)
	}
	req.Remark = "updated"
	req.Details[0].ExpectedQty = 7
	if err := s.in.Update(ctx, order.ID, &req); err != nil {
		t.Fatalf("single connection update: %v", err)
	}
	req.Remark = "must rollback"
	req.Details[0].SKUID = -1
	if err := s.in.Update(ctx, order.ID, &req); !errors.Is(err, errcode.SKUNotFound) {
		t.Fatalf("invalid SKU: %v", err)
	}
	detail, err := s.in.Get(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Order.Remark != "updated" || detail.Order.ExpectedQty != 7 || len(detail.Details) != 1 || detail.Details[0].SKUID != s.skuID {
		t.Fatalf("invalid edit changed order: %+v", detail)
	}
	if pool.Stats().WaitCount != 0 {
		t.Fatalf("unexpected second connection wait: %+v", pool.Stats())
	}
}

type orderReferenceCommandKey struct{}

func TestOrderCreationSerializesWithSKUDeletion(t *testing.T) {
	for _, kind := range []string{"inbound", "outbound"} {
		for _, deleteFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deleteFirst=%v", kind, deleteFirst), func(t *testing.T) {
				s := newOrderReferenceStack(t)
				ctx, cancel := context.WithCancel(s.ctx)
				s.ctx = ctx
				t.Cleanup(cancel)
				arrived, release := make(chan struct{}), make(chan struct{})
				var once, releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				t.Cleanup(unblock)
				gate := func() {
					once.Do(func() {
						close(arrived)
						select {
						case <-release:
						case <-s.ctx.Done():
						}
					})
				}
				const name = "p1:order_reference_gate"
				if deleteFirst {
					if err := s.db.Callback().Delete().Before("gorm:delete").Register(name, func(q *gorm.DB) {
						if q.Statement.Table == "wms_sku" {
							gate()
						}
					}); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = s.db.Callback().Delete().Remove(name) })
				} else {
					table := "wms_receipt_order"
					if kind == "outbound" {
						table = "wms_shipment_order"
					}
					if err := s.db.Callback().Create().Before("gorm:create").Register(name, func(q *gorm.DB) {
						if q.Statement.Table == table {
							gate()
						}
					}); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = s.db.Callback().Create().Remove(name) })
				}
				attempted := make(chan struct{})
				var attemptOnce sync.Once
				if err := s.db.Callback().Query().Before("gorm:query").Register("p1:second_reference_attempt", func(q *gorm.DB) {
					_, locking := q.Statement.Clauses["FOR"]
					if q.Statement.Table == "wms_sku" && locking && q.Statement.Context.Value(orderReferenceCommandKey{}) == "second" {
						attemptOnce.Do(func() { close(attempted) })
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = s.db.Callback().Query().Remove("p1:second_reference_attempt") })
				first, second := make(chan error, 1), make(chan error, 1)
				create := func(ctx context.Context) error { copy := *s; copy.ctx = ctx; return copy.create(kind) }
				remove := func(ctx context.Context) error { return s.basic.DeleteSKU(ctx, s.skuID) }
				firstOp, secondOp := create, remove
				if deleteFirst {
					firstOp, secondOp = remove, create
				}
				var workers sync.WaitGroup
				t.Cleanup(func() { unblock(); cancel(); workers.Wait() })
				workers.Add(1)
				go func() { defer workers.Done(); first <- firstOp(ctx) }()
				waitClosed(t, "first holds reference lock", arrived)
				workers.Add(1)
				go func() {
					defer workers.Done()
					second <- secondOp(context.WithValue(ctx, orderReferenceCommandKey{}, "second"))
				}()
				select {
				case <-attempted:
				case <-ctx.Done():
					t.Fatal("second command did not attempt reference lock")
				}
				// With the shared row lock the second command cannot finish before
				// the first transaction commits. The old code deletes/creates here.
				select {
				case err := <-second:
					t.Fatalf("second command bypassed reference lock: %v", err)
				case <-time.After(100 * time.Millisecond):
				}
				unblock()
				if err := waitError(t, "first", first); err != nil {
					t.Fatalf("first: %v", err)
				}
				want := errcode.SKUHasReferences
				if deleteFirst {
					want = errcode.SKUNotFound
				}
				if err := waitError(t, "second", second); !errors.Is(err, want) {
					t.Fatalf("second=%v want=%v", err, want)
				}
				var count int64
				model := any(&inmodel.ReceiptOrder{})
				if kind == "outbound" {
					model = &outmodel.ShipmentOrder{}
				}
				if err := s.db.WithContext(s.ctx).Model(model).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				wantCount := int64(1)
				if deleteFirst {
					wantCount = 0
				}
				if count != wantCount {
					t.Fatalf("orders=%d want=%d", count, wantCount)
				}
			})
		}
	}
}

func TestOrderReferenceLocksPreserveTenantIsolation(t *testing.T) {
	s := newOrderReferenceStack(t)
	s.ctx = tenant.WithTenant(s.ctx, 82)
	for _, kind := range []string{"inbound", "outbound"} {
		if err := s.create(kind); !errors.Is(err, errcode.WarehouseNotFound) {
			t.Fatalf("%s cross tenant: %v", kind, err)
		}
	}
}
