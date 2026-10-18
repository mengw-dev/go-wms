package migrations_test

import (
	"context"
	"errors"
	"os"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	"gowms/internal/bootstrap"
	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/api"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/quantity"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
	"gowms/migrations"
)

func TestQuantitySchemaAndBoundariesMatchBothCreationPaths(t *testing.T) {
	for _, formal := range []bool{false, true} {
		name := "AutoMigrate"
		if formal {
			name = "versioned SQL"
		}
		t.Run(name, func(t *testing.T) {
			dsn := os.Getenv("WMS_TEST_DSN")
			if dsn == "" {
				dsn = "root:1234@tcp(127.0.0.1:3306)/?parseTime=true&timeout=2s"
			}
			cfg, err := mysqldriver.ParseDSN(dsn)
			if err != nil {
				t.Fatal(err)
			}
			cfg.MultiStatements = true
			db := testutil.OpenIsolatedMySQL(t, cfg.FormatDSN())
			if formal {
				sqlDB, err := db.DB()
				if err != nil {
					t.Fatal(err)
				}
				driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
				if err != nil {
					t.Fatal(err)
				}
				source, err := iofs.New(migrations.FS, "versions")
				if err != nil {
					t.Fatal(err)
				}
				m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
						t.Errorf("close migrate: %v %v", srcErr, dbErr)
					}
				})
				if err := m.Up(); err != nil {
					t.Fatal(err)
				}
			} else if err := bootstrap.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			var columns []struct{ TableName, ColumnName, DataType string }
			if err := db.Raw(`SELECT table_name, column_name, data_type FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name IN ('wms_inventory', 'wms_inventory_trans', 'wms_task', 'wms_receipt_order', 'wms_receipt_order_detail', 'wms_shipment_order', 'wms_shipment_order_detail', 'wms_allocation', 'wms_stocktake_detail')
				AND column_name IN ('stock_quantity', 'available_quantity', 'allocated_quantity', 'quantity_change', 'before_quantity', 'after_quantity', 'available_before', 'available_after', 'target_qty', 'done_qty', 'expected_qty', 'received_qty', 'defective_qty', 'allocated_qty', 'picked_qty', 'book_qty', 'actual_qty', 'diff_qty')`).Scan(&columns).Error; err != nil {
				t.Fatal(err)
			}
			if len(columns) != 27 {
				t.Fatalf("missing quantity columns: count=%d", len(columns))
			}
			for _, column := range columns {
				if column.DataType != "int" {
					t.Errorf("%s.%s: %s, want int", column.TableName, column.ColumnName, column.DataType)
				}
			}
			if err := tenant.RegisterGORMCallbacks(db); err != nil {
				t.Fatal(err)
			}
			if err := snowflake.Init(1); err != nil {
				t.Fatal(err)
			}
			ctx := tenant.WithTenant(context.Background(), 11)
			for _, row := range []any{
				&basicmodel.Warehouse{Base: modelbase.Base{ID: 1}, Code: "W", Name: "warehouse", Status: 1},
				&basicmodel.Location{Base: modelbase.Base{ID: 1}, WarehouseID: 1, Code: "L", Status: 1},
				&basicmodel.SKU{Base: modelbase.Base{ID: 1}, Code: "S", Barcode: "B", Name: "sku", Status: 1},
			} {
				if err := db.WithContext(ctx).Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			tm := tx.New(db)
			s := invservice.New(invrepo.New(), tm)
			increase := func(qty int) error {
				return tm.Tx(ctx, func(q *gorm.DB) error {
					return s.Increase(ctx, q, &api.IncreaseReq{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B", Quantity: qty})
				})
			}
			if err := increase(quantity.Max); err != nil {
				t.Fatalf("INT max rejected: %v", err)
			}
			if err := increase(1); !errors.Is(err, errcode.QuantityLimitExceeded) {
				t.Fatalf("cumulative overflow was not a parameter error: %v", err)
			}
			if err := increase(quantity.Max + 1); !errors.Is(err, errcode.QuantityLimitExceeded) {
				t.Fatalf("request overflow accepted: %v", err)
			}
			var stock invmodel.Inventory
			if err := db.WithContext(ctx).First(&stock).Error; err != nil || stock.StockQuantity != quantity.Max || stock.AvailableQty != quantity.Max {
				t.Fatalf("rejected increase mutated inventory: %+v %v", stock, err)
			}
			if err := tm.Tx(ctx, func(q *gorm.DB) error {
				allocation, err := s.Allocate(ctx, q, &api.AllocateReq{WarehouseID: 1, SKUID: 1, Quantity: quantity.Max})
				if err != nil {
					return err
				}
				return s.Ship(ctx, q, &api.ShipReq{InventoryID: allocation.Rows[0].InventoryID, Quantity: quantity.Max})
			}); err != nil {
				t.Fatalf("INT boundary allocate/ship failed: %v", err)
			}
		})
	}
}
