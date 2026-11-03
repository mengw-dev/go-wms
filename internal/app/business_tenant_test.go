package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	basicdto "gowms/internal/modules/basic/dto"
	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	indto "gowms/internal/modules/inbound/dto"
	inrepo "gowms/internal/modules/inbound/repository"
	inservice "gowms/internal/modules/inbound/service"
	invapi "gowms/internal/modules/inventory/api"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	outdto "gowms/internal/modules/outbound/dto"
	outrepo "gowms/internal/modules/outbound/repository"
	outservice "gowms/internal/modules/outbound/service"
	stockdto "gowms/internal/modules/stocktake/dto"
	stockrepo "gowms/internal/modules/stocktake/repository"
	stockservice "gowms/internal/modules/stocktake/service"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

type businessTenantStack struct {
	db    *gorm.DB
	basic *basicservice.Service
	in    *inservice.Service
	out   *outservice.Service
	stock *stockservice.Service
	inv   *invservice.Service
	tasks *taskservice.Service
}

func newBusinessTenantStack(t *testing.T) *businessTenantStack {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenMigratedMySQL(t, dsn)
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}
	tm := tx.New(db)
	inv := invservice.New(invrepo.New(), tm)
	basic := basicservice.New(basicrepo.New(), tm, nil, inv, config.LimitsConfig{})
	tasks := taskservice.New(taskrepo.New(), db)
	return &businessTenantStack{
		db: db, basic: basic, inv: inv, tasks: tasks,
		in:    inservice.New(inrepo.New(), tm, orderno.New(nil), basic, inv, tasks, config.UploadConfig{}, config.LimitsConfig{}),
		out:   outservice.New(outrepo.New(), tm, orderno.New(nil), basic, inv, tasks, config.LimitsConfig{}),
		stock: stockservice.New(stockrepo.New(), tm, orderno.New(nil), inv, config.LimitsConfig{}),
	}
}

func requireBusinessError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

// Exercise the same real business chain for tenant zero and an ordinary tenant.
// Deliberately use WithTenant(0), which still permits platform read queries.
func TestBusinessCommandsUseOwnTenant(t *testing.T) {
	for _, tid := range []int64{11, 0} {
		t.Run(fmt.Sprintf("tenant-%d", tid), func(t *testing.T) {
			s := newBusinessTenantStack(t)
			ctx := tenant.WithTenant(context.Background(), tid)
			foreignID := int64(11)
			if tid == 11 {
				foreignID = 0
			}
			foreign := tenant.WithTenant(context.Background(), foreignID)
			// Identical codes in another tenant must not block this tenant's creation.
			for _, row := range []any{
				&basicmodel.Warehouse{Base: modelbase.Base{ID: 1}, Code: "WH", Name: "foreign", Status: 1},
				&basicmodel.SKU{Base: modelbase.Base{ID: 1}, Code: "SKU", Barcode: "BC", Name: "foreign", Status: 1},
			} {
				requireBusinessError(t, s.db.WithContext(tenant.WithExactTenant(foreign, foreignID)).Create(row).Error, nil)
			}
			requireBusinessError(t, s.basic.CreateWarehouse(ctx, &basicdto.WarehouseReq{Code: "WH", Name: "own"}), nil)
			requireBusinessError(t, s.basic.CreateSKU(ctx, &basicdto.SKUReq{Code: "SKU", Barcode: "BC", Name: "own", Unit: "件"}), nil)
			exact := tenant.WithExactTenant(ctx, tid)
			wh, err := s.basic.GetWarehouseByCode(exact, "WH")
			requireBusinessError(t, err, nil)
			sku, err := s.basic.GetSKUByCode(exact, "SKU")
			requireBusinessError(t, err, nil)
			locReq := &basicdto.LocationBatchReq{WarehouseID: wh.ID, Zone: "A", RowFrom: 1, RowTo: 1, ColFrom: 1, ColTo: 1}
			_, err = s.basic.BatchCreateLocations(foreign, locReq)
			requireBusinessError(t, err, errcode.WarehouseNotFound)
			n, err := s.basic.BatchCreateLocations(ctx, locReq)
			requireBusinessError(t, err, nil)
			if n != 1 {
				t.Fatalf("created locations = %d", n)
			}
			locs, _, err := s.basic.ListLocations(exact, &basicdto.LocationQuery{WarehouseID: wh.ID, CommonQuery: basicdto.CommonQuery{Page: 1, PageSize: 10}})
			requireBusinessError(t, err, nil)
			if len(locs) != 1 || locs[0].TenantID != tid {
				t.Fatalf("locations = %+v", locs)
			}
			loc := locs[0]
			for _, tc := range []struct {
				name string
				run  func() error
				want error
			}{
				{"warehouse edit", func() error { return s.basic.UpdateWarehouse(foreign, wh.ID, &basicdto.WarehouseReq{Name: "wrong"}) }, errcode.WarehouseNotFound},
				{"warehouse disable", func() error { return s.basic.UpdateWarehouseStatus(foreign, wh.ID, 0) }, errcode.WarehouseNotFound},
				{"warehouse delete", func() error { return s.basic.DeleteWarehouse(foreign, wh.ID) }, errcode.WarehouseNotFound},
				{"sku edit", func() error {
					return s.basic.UpdateSKU(foreign, sku.ID, &basicdto.SKUReq{Name: "wrong", Barcode: "wrong"})
				}, errcode.SKUNotFound},
				{"sku delete", func() error { return s.basic.DeleteSKU(foreign, sku.ID) }, errcode.SKUNotFound},
				{"location disable", func() error { return s.basic.UpdateLocationStatus(foreign, loc.ID, 0) }, errcode.LocationNotFound},
				{"location delete", func() error { return s.basic.DeleteLocation(foreign, loc.ID) }, errcode.LocationNotFound},
			} {
				t.Run(tc.name, func(t *testing.T) { requireBusinessError(t, tc.run(), tc.want) })
			}

			inReq := &indto.CreateOrderReq{WarehouseID: wh.ID, Details: []indto.OrderDetailItem{{SKUID: sku.ID, ExpectedQty: 10}}}
			_, err = s.in.Create(ctx, &indto.CreateOrderReq{WarehouseID: wh.ID, Details: []indto.OrderDetailItem{{SKUID: 1, ExpectedQty: 1}}}, "owner")
			requireBusinessError(t, err, errcode.SKUNotFound)
			_, err = s.in.Create(foreign, inReq, "foreign")
			requireBusinessError(t, err, errcode.WarehouseNotFound)
			in, err := s.in.Create(ctx, inReq, "owner")
			requireBusinessError(t, err, nil)
			// Platform reads retain their cross-tenant visibility.
			_, err = s.in.Get(context.Background(), in.ID)
			requireBusinessError(t, err, nil)
			requireBusinessError(t, s.in.Update(foreign, in.ID, &indto.UpdateOrderReq{CreateOrderReq: *inReq, ExpectedVersion: in.Version}), errcode.OrderNotFound)
			requireBusinessError(t, s.in.Delete(foreign, in.ID), errcode.OrderNotFound)
			requireBusinessError(t, s.in.Submit(foreign, in.ID), errcode.OrderNotFound)
			requireBusinessError(t, s.in.Submit(ctx, in.ID), nil)
			requireBusinessError(t, s.in.Approve(foreign, in.ID, "foreign"), errcode.OrderNotFound)
			requireBusinessError(t, s.in.Cancel(foreign, in.ID), errcode.OrderNotFound)
			requireBusinessError(t, s.in.Approve(ctx, in.ID, "owner"), nil)
			detail, err := s.in.Get(ctx, in.ID)
			requireBusinessError(t, err, nil)
			receive := &indto.ReceiveReq{Qty: 10, BatchNo: "B1"}
			requireBusinessError(t, s.in.Receive(foreign, in.ID, detail.Details[0].ID, receive, "foreign", "receive"), errcode.OrderNotFound)
			for range 2 {
				requireBusinessError(t, s.in.Receive(ctx, in.ID, detail.Details[0].ID, receive, "owner", "receive"), nil)
			}
			detail, err = s.in.Get(ctx, in.ID)
			requireBusinessError(t, err, nil)
			var putawayID int64
			for _, task := range detail.Tasks {
				if task.TaskType == taskmodel.TaskPutaway {
					putawayID = task.ID
				}
			}
			if putawayID == 0 {
				t.Fatal("missing putaway task")
			}
			requireBusinessError(t, s.in.Putaway(foreign, putawayID, loc.ID, 10, "foreign", "putaway"), errcode.TaskNotFound)
			for range 2 {
				requireBusinessError(t, s.in.Putaway(ctx, putawayID, loc.ID, 10, "owner", "putaway"), nil)
			}

			outReq := &outdto.CreateOrderReq{WarehouseID: wh.ID, BizOrderNo: "BIZ", Details: []outdto.OrderDetailItem{{SKUID: sku.ID, ExpectedQty: 4}}}
			_, err = s.out.Create(ctx, &outdto.CreateOrderReq{WarehouseID: wh.ID, BizOrderNo: "BAD-SKU", Details: []outdto.OrderDetailItem{{SKUID: 1, ExpectedQty: 1}}}, "owner")
			requireBusinessError(t, err, errcode.SKUNotFound)
			_, err = s.out.Create(foreign, outReq, "foreign")
			requireBusinessError(t, err, errcode.WarehouseNotFound)
			out, err := s.out.Create(ctx, outReq, "owner")
			requireBusinessError(t, err, nil)
			requireBusinessError(t, s.out.Delete(foreign, out.ID), errcode.ShipOrderNotFound)
			requireBusinessError(t, s.out.Submit(foreign, out.ID), errcode.ShipOrderNotFound)
			requireBusinessError(t, s.out.Submit(ctx, out.ID), nil)
			requireBusinessError(t, s.out.Approve(foreign, out.ID, "foreign"), errcode.ShipOrderNotFound)
			requireBusinessError(t, s.out.Approve(ctx, out.ID, "owner"), nil)
			requireBusinessError(t, s.out.Cancel(foreign, out.ID, "foreign"), errcode.ShipOrderNotFound)
			outDetail, err := s.out.Get(ctx, out.ID)
			requireBusinessError(t, err, nil)
			if len(outDetail.Tasks) != 1 {
				t.Fatalf("pick tasks = %d", len(outDetail.Tasks))
			}
			pickID := outDetail.Tasks[0].ID
			_, err = s.out.ClaimPickTask(foreign, pickID, "foreign", "claim")
			requireBusinessError(t, err, errcode.TaskNotFound)
			claim, err := s.out.ClaimPickTask(ctx, pickID, "owner", "claim")
			requireBusinessError(t, err, nil)
			_, err = s.out.Pick(foreign, pickID, 4, "foreign", nil, "", "pick")
			requireBusinessError(t, err, errcode.TaskNotFound)
			for range 2 {
				_, err = s.out.Pick(ctx, pickID, 4, "owner", nil, claim.ClaimToken, "pick")
				requireBusinessError(t, err, nil)
			}

			_, err = s.stock.Create(foreign, &stockdto.CreateOrderReq{WarehouseID: wh.ID}, "foreign")
			requireBusinessError(t, err, errcode.StocktakeNoDetail)
			count, err := s.stock.Create(ctx, &stockdto.CreateOrderReq{WarehouseID: wh.ID}, "owner")
			requireBusinessError(t, err, nil)
			countDetail, err := s.stock.Get(ctx, count.ID)
			requireBusinessError(t, err, nil)
			if len(countDetail.Details) != 1 {
				t.Fatalf("stocktake details = %d", len(countDetail.Details))
			}
			countID := countDetail.Details[0].ID
			requireBusinessError(t, s.stock.RecordActual(foreign, count.ID, countID, 5), errcode.StocktakeNotFound)
			requireBusinessError(t, s.stock.Cancel(foreign, count.ID), errcode.StocktakeNotFound)
			requireBusinessError(t, s.stock.RecordActual(ctx, count.ID, countID, 5), nil)
			requireBusinessError(t, s.stock.Approve(foreign, count.ID, "foreign", "approve"), errcode.StocktakeNotFound)
			for range 2 {
				requireBusinessError(t, s.stock.Approve(ctx, count.ID, "owner", "approve"), nil)
			}
			var inventory invmodel.Inventory
			requireBusinessError(t, s.db.WithContext(exact).First(&inventory).Error, nil)
			if inventory.StockQuantity != 5 || inventory.AvailableQty != 5 || inventory.AllocatedQty != 0 || inventory.TenantID != tid {
				t.Fatalf("inventory = %+v", inventory)
			}
			var ledger []invmodel.InventoryTrans
			requireBusinessError(t, s.db.Find(&ledger).Error, nil)
			if len(ledger) != 4 {
				t.Fatalf("ledger rows = %d, want RECEIVE/ALLOCATE/SHIP/ADJUST once", len(ledger))
			}
			for _, row := range ledger {
				if row.TenantID != tid || row.InventoryID != inventory.ID {
					t.Fatalf("wrong ledger owner: %+v", row)
				}
			}
			var keys []idempotency.Record
			requireBusinessError(t, s.db.Find(&keys).Error, nil)
			if len(keys) != 5 {
				t.Fatalf("idempotency records = %d", len(keys))
			}
			for _, row := range keys {
				if row.TenantID != tid {
					t.Fatalf("wrong idempotency owner: %+v", row)
				}
			}
		})
	}
}

// Internal APIs must honor their explicit business context even when their
// transaction was opened by a platform caller. WithContext must keep that tx.
func TestInventoryAPIUsesExplicitTenantAndTransaction(t *testing.T) {
	s := newBusinessTenantStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	owner := tenant.WithTenant(ctx, 11)
	for _, row := range []any{
		&basicmodel.Warehouse{Base: modelbase.Base{ID: 1}, Code: "WH", Name: "WH", Status: 1},
		&basicmodel.Location{Base: modelbase.Base{ID: 1}, WarehouseID: 1, Code: "A", Status: 1},
		&basicmodel.SKU{Base: modelbase.Base{ID: 1}, Code: "SKU", Barcode: "BC", Name: "SKU", Status: 1},
	} {
		requireBusinessError(t, s.db.WithContext(owner).Create(row).Error, nil)
	}
	pool, err := s.db.DB()
	requireBusinessError(t, err, nil)
	// The migration fixture holds one dedicated connection until cleanup;
	// leave exactly one other connection for the business transaction.
	pool.SetMaxOpenConns(2)
	rolledBack := errors.New("rollback probe")
	err = s.db.WithContext(ctx).Transaction(func(db *gorm.DB) error {
		requireBusinessError(t, s.inv.Increase(owner, db, &invapi.IncreaseReq{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B", Quantity: 10}), nil)
		var row invmodel.Inventory
		requireBusinessError(t, db.First(&row).Error, nil)
		if row.TenantID != 11 {
			t.Fatalf("inventory belongs to %d", row.TenantID)
		}
		platform := ctx
		requireBusinessError(t, s.basic.UpdateLocationStatusInTx(platform, db, 1, 0), errcode.LocationNotFound)
		requireBusinessError(t, s.inv.Increase(platform, db, &invapi.IncreaseReq{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B", Quantity: 1}), gorm.ErrRecordNotFound)
		_, err := s.inv.Allocate(platform, db, &invapi.AllocateReq{WarehouseID: 1, SKUID: 1, Quantity: 1})
		if errcode.From(err).Code != errcode.AvailableNotEnough.Code {
			t.Fatalf("foreign allocation: %v", err)
		}
		_, err = s.inv.Adjust(platform, db, &invapi.AdjustReq{InventoryID: row.ID, NewStock: 8})
		requireBusinessError(t, err, errcode.InventoryNotFound)
		requireBusinessError(t, s.inv.Freeze(platform, db, &invapi.FreezeReq{InventoryID: row.ID, Quantity: 1}), errcode.InventoryNotFound)
		requireBusinessError(t, s.inv.WriteOff(platform, db, &invapi.FreezeReq{InventoryID: row.ID, Quantity: 1}), gorm.ErrRecordNotFound)
		requireBusinessError(t, s.inv.ReceiveReturn(platform, db, &invapi.ReturnReq{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B", GoodQty: 1}), gorm.ErrRecordNotFound)
		_, err = s.inv.Allocate(owner, db, &invapi.AllocateReq{WarehouseID: 1, SKUID: 1, Quantity: 2})
		requireBusinessError(t, err, nil)
		requireBusinessError(t, s.inv.Ship(platform, db, &invapi.ShipReq{InventoryID: row.ID, Quantity: 1}), errcode.InventoryNotFound)
		requireBusinessError(t, s.inv.Release(platform, db, &invapi.ReleaseReq{InventoryID: row.ID, Quantity: 1}), errcode.InventoryNotFound)
		var ledger []invmodel.InventoryTrans
		requireBusinessError(t, db.Find(&ledger).Error, nil)
		for _, entry := range ledger {
			if entry.TenantID != 11 {
				t.Fatalf("ledger belongs to %d", entry.TenantID)
			}
		}
		return rolledBack
	})
	requireBusinessError(t, err, rolledBack)
	for _, model := range []any{&invmodel.Inventory{}, &invmodel.InventoryTrans{}} {
		var n int64
		requireBusinessError(t, s.db.WithContext(ctx).Model(model).Count(&n).Error, nil)
		if n != 0 {
			t.Fatalf("rollback left %d rows for %T", n, model)
		}
	}
}
