package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/modules/stocktake/repository"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func stocktakeFixture(t *testing.T) (*Service, *gorm.DB, context.Context, *invmodel.Inventory) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &basicmodel.SKU{}, &basicmodel.Location{}, &invmodel.Inventory{}, &invmodel.InventoryTrans{}, &model.StocktakeOrder{}, &model.StocktakeDetail{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithTenant(context.Background(), 11)
	for _, row := range []any{
		&basicmodel.SKU{Base: sysmodel.Base{ID: 1}, Code: "SKU", Barcode: "BAR", Name: "Item"},
		&basicmodel.Location{Base: sysmodel.Base{ID: 1}, WarehouseID: 1, Code: "A01"},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	inventory := &invmodel.Inventory{Base: sysmodel.Base{ID: 1}, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B1", StockQuantity: 10, AvailableQty: 10, StockInTime: time.Now()}
	if err := db.WithContext(ctx).Create(inventory).Error; err != nil {
		t.Fatal(err)
	}
	tm := tx.New(db)
	return New(repository.New(), tm, orderno.New(nil), invservice.New(invrepo.New(), tm), config.LimitsConfig{}), db, ctx, inventory
}

func countedOrder(ctx context.Context, t *testing.T, s *Service, actual int) (*model.StocktakeOrder, int64) {
	t.Helper()
	o, err := s.Create(ctx, &dto.CreateOrderReq{WarehouseID: 1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.Get(ctx, o.ID)
	if err != nil || len(detail.Details) != 1 {
		t.Fatalf("details=%+v err=%v", detail, err)
	}
	id := detail.Details[0].ID
	if err := s.RecordActual(ctx, o.ID, id, actual); err != nil {
		t.Fatal(err)
	}
	return o, id
}

func TestStocktakeSnapshotTenantIsolation(t *testing.T) {
	s, _, ctx, _ := stocktakeFixture(t)
	if _, err := s.Create(tenant.WithTenant(ctx, 22), &dto.CreateOrderReq{WarehouseID: 1}, "other"); !errors.Is(err, errcode.StocktakeNoDetail) {
		t.Fatalf("cross tenant snapshot: %v", err)
	}
	o, err := s.Create(ctx, &dto.CreateOrderReq{WarehouseID: 1, LocationID: 1, LocationCode: "forged"}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if o.LocationCode != "A01" {
		t.Fatalf("location code trusted request: %s", o.LocationCode)
	}
}

func TestStocktakeRollbackAndMissingInventory(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "allocated stock", true: "deleted stock"}[missing], func(t *testing.T) {
			s, db, ctx, inventory := stocktakeFixture(t)
			o, id := countedOrder(ctx, t, s, 0)
			want := errcode.AdjustNotAllow
			if missing {
				if err := db.WithContext(ctx).Delete(inventory).Error; err != nil {
					t.Fatal(err)
				}
				want = errcode.InventoryNotFound
			} else {
				if err := db.Model(inventory).Updates(map[string]any{"available_quantity": 2, "allocated_quantity": 8}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := s.Approve(ctx, o.ID, "test"); !errors.Is(err, want) {
				t.Fatalf("got %v want %v", err, want)
			}
			result, err := s.Get(ctx, o.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result.Order.Status != model.OrderDraft || result.Details[0].Adjusted {
				t.Fatal("failed approval changed order or detail")
			}
			if err := s.RecordActual(ctx, o.ID, id, -1); !errors.Is(err, errcode.StocktakeQtyInvalid) {
				t.Fatalf("negative count: %v", err)
			}
		})
	}
}

func TestStocktakeDifferenceUsesLockedCurrentStock(t *testing.T) {
	s, db, ctx, inventory := stocktakeFixture(t)
	o, _ := countedOrder(ctx, t, s, 8)
	blocker := db.WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer blocker.Rollback()
	if err := blocker.Clauses(clause.Locking{Strength: "UPDATE"}).First(&invmodel.Inventory{}, inventory.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := blocker.Model(inventory).Updates(map[string]any{"stock_quantity": 12, "available_quantity": 12}).Error; err != nil {
		t.Fatal(err)
	}
	locking := make(chan struct{}, 1)
	if err := db.Callback().Query().Before("gorm:query").Register("test:inventory_lock", func(db *gorm.DB) {
		if db.Statement.Table == "wms_inventory" {
			select {
			case locking <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:inventory_lock") })
	done := make(chan error, 1)
	go func() { done <- s.Approve(ctx, o.ID, "test") }()
	select {
	case <-locking:
	case <-time.After(5 * time.Second):
		t.Fatal("approval did not reach inventory lock")
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("approval stuck")
	}
	result, err := s.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	var flow invmodel.InventoryTrans
	if err := db.Where("order_no = ?", o.OrderNo).First(&flow).Error; err != nil {
		t.Fatal(err)
	}
	if result.Details[0].DiffQty != -4 || flow.QuantityChange != -4 || flow.BeforeQuantity != 12 || flow.AfterQuantity != 8 {
		t.Fatalf("detail=%+v flow=%+v", result.Details[0], flow)
	}
	if err := s.Cancel(ctx, o.ID); !errors.Is(err, errcode.StocktakeStatusWrong) {
		t.Fatalf("completed order cancellation: %v", err)
	}
}
