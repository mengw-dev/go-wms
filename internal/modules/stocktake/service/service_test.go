package service

import (
	"context"
	"errors"
	"fmt"
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

// 未盘完的盘点单必须整单拒绝：不调整库存、不写流水、不推进单据状态；
// 补齐全部实盘数量后可以正常完成审核。
func TestStocktakeApproveRequiresFullyCountedDetails(t *testing.T) {
	s, db, ctx, _ := stocktakeFixture(t)
	// 补两条库存，让整仓快照生成 3 条盘点明细。
	for i := int64(2); i <= 3; i++ {
		if err := db.WithContext(ctx).Create(&basicmodel.SKU{
			Base: sysmodel.Base{ID: i}, Code: fmt.Sprintf("SKU-%d", i),
			Barcode: fmt.Sprintf("BAR-%d", i), Name: "Item",
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(ctx).Create(&invmodel.Inventory{
			Base: sysmodel.Base{ID: i}, WarehouseID: 1, LocationID: 1, SKUID: i,
			BatchNo: "B1", StockQuantity: 10, AvailableQty: 10, StockInTime: time.Now(),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	o, err := s.Create(ctx, &dto.CreateOrderReq{WarehouseID: 1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	detail, err := s.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Details) != 3 {
		t.Fatalf("details=%d want 3", len(detail.Details))
	}

	// 只录入前两条实盘数，第三条保持未盘。
	actualByInventory := make(map[int64]int, len(detail.Details))
	for _, d := range detail.Details[:2] {
		if err := s.RecordActual(ctx, o.ID, d.ID, 8); err != nil {
			t.Fatal(err)
		}
		actualByInventory[d.InventoryID] = 8
	}
	actualByInventory[detail.Details[2].InventoryID] = 12

	if err := s.Approve(ctx, o.ID, "test"); !errors.Is(err, errcode.StocktakeNotFullyCounted) {
		t.Fatalf("approve with missing actual qty: %v", err)
	}
	// 拒绝后：单据仍是草稿，明细未被调整，库存三数量和流水完全不变。
	result, err := s.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Order.Status != model.OrderDraft {
		t.Fatalf("status after rejected approval = %s", result.Order.Status)
	}
	for _, d := range result.Details {
		if d.Adjusted || d.DiffQty != 0 {
			t.Fatalf("detail changed by rejected approval: %+v", d)
		}
	}
	var inventories []*invmodel.Inventory
	if err := db.WithContext(ctx).Order("id").Find(&inventories).Error; err != nil {
		t.Fatal(err)
	}
	for _, inv := range inventories {
		if inv.StockQuantity != 10 || inv.AvailableQty != 10 || inv.AllocatedQty != 0 {
			t.Fatalf("inventory changed by rejected approval: %+v", inv)
		}
	}
	if n := countStocktakeAdjustTrans(ctx, t, db, o.OrderNo); n != 0 {
		t.Fatalf("adjust trans after rejected approval = %d want 0", n)
	}

	// 补录第三条实盘数后审核成功：库存按实盘数调整，每条明细写入一条 ADJUST 流水。
	if err := s.RecordActual(ctx, o.ID, detail.Details[2].ID, 12); err != nil {
		t.Fatal(err)
	}
	if err := s.Approve(ctx, o.ID, "test"); err != nil {
		t.Fatal(err)
	}
	result, err = s.Get(ctx, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Order.Status != model.OrderCompleted {
		t.Fatalf("status after approval = %s", result.Order.Status)
	}
	for _, d := range result.Details {
		if !d.Adjusted || d.ActualQty == nil || *d.ActualQty != d.BookQty+d.DiffQty {
			t.Fatalf("detail not adjusted: %+v", d)
		}
	}
	var afterApproval []*invmodel.Inventory
	if err := db.WithContext(ctx).Order("id").Find(&afterApproval).Error; err != nil {
		t.Fatal(err)
	}
	for _, inv := range afterApproval {
		want := actualByInventory[inv.ID]
		if inv.StockQuantity != want || inv.AvailableQty != want || inv.AllocatedQty != 0 {
			t.Fatalf("inventory after approval: %+v want stock %d", inv, want)
		}
	}
	if n := countStocktakeAdjustTrans(ctx, t, db, o.OrderNo); n != int64(len(result.Details)) {
		t.Fatalf("adjust trans after approval = %d want %d", n, len(result.Details))
	}
}

func countStocktakeAdjustTrans(ctx context.Context, t *testing.T, db *gorm.DB, orderNo string) int64 {
	t.Helper()
	var n int64
	err := db.WithContext(ctx).Model(&invmodel.InventoryTrans{}).
		Where("order_no = ? AND trans_type = ?", orderNo, invmodel.TransAdjust).Count(&n).Error
	if err != nil {
		t.Fatal(err)
	}
	return n
}
