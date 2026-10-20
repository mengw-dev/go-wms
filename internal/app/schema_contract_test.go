package app

import (
	"context"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/bootstrap"
	basicmodel "gowms/internal/modules/basic/model"
	demoservice "gowms/internal/modules/demo/service"
	inmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

// Exercise InitDB + app.New against both actual creation paths. Partial model
// fixtures remain useful for fault injection; they are not release-schema proof.
func TestApplicationSchemaAndTenantContract(t *testing.T) {
	var developmentColumns map[string]string
	for _, path := range []string{"development", "release"} {
		t.Run(path, func(t *testing.T) {
			dsn := os.Getenv("WMS_TEST_DSN")
			if dsn == "" {
				dsn = "root:1234@tcp(127.0.0.1:3306)/?parseTime=true&timeout=2s"
			}
			var isolated *gorm.DB
			if path == "release" {
				isolated = testutil.OpenMigratedMySQL(t, dsn)
			} else {
				isolated = testutil.OpenIsolatedMySQL(t, dsn)
				if err := bootstrap.AutoMigrate(isolated); err != nil {
					t.Fatal(err)
				}
			}
			var name string
			if err := isolated.Raw("SELECT DATABASE()").Scan(&name).Error; err != nil {
				t.Fatal(err)
			}
			mysqlConfig, err := mysqldriver.ParseDSN(dsn)
			if err != nil {
				t.Fatal(err)
			}
			mysqlConfig.DBName, mysqlConfig.ParseTime = name, true
			cfg := &config.Config{
				Server: config.ServerConfig{Mode: "release"},
				MySQL:  config.MySQLConfig{DSN: mysqlConfig.FormatDSN(), MaxOpenConns: 8, MaxIdleConns: 2},
				Demo:   config.DemoConfig{Enabled: true, Instances: 1, SessionTTLSeconds: 300},
			}
			db, err := bootstrap.InitDB(cfg)
			if err != nil {
				t.Fatal(err)
			}
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pool.Close() })
			if err := bootstrap.VerifyQuantityChecks(db); err != nil {
				t.Fatal(err)
			}
			columns := schemaColumnContract(t, db)
			if path == "development" {
				developmentColumns = columns
			} else if developmentColumns == nil || !maps.Equal(developmentColumns, columns) {
				for key, typ := range developmentColumns {
					if columns[key] != typ {
						t.Errorf("schema contract %s: development=%q release=%q", key, typ, columns[key])
					}
				}
				for key := range columns {
					if _, exists := developmentColumns[key]; !exists {
						t.Errorf("release-only column: %s", key)
					}
				}
			}
			assertTenantUniqueIndexes(t, db)
			if err := snowflake.Init(1); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			ctx = tenant.WithTenant(ctx, 10001)
			app := New(cfg, db, testutil.OpenTestRedis(t), nil)
			session, err := app.DemoService.AcquireSession(ctx)
			if err != nil {
				t.Fatal(err)
			}
			result, err := app.DemoService.RunScenario(ctx, session.SessionID, demoservice.ScenarioFull)
			if err != nil || result == nil || result.Status != demoservice.ScenarioStatusCompleted {
				t.Fatalf("real business chain: result=%+v err=%v", result, err)
			}
			for _, step := range result.Steps {
				if step.Status != demoservice.ScenarioStepCompleted {
					t.Fatalf("incomplete real step: %+v", step)
				}
			}
			for _, model := range []any{&inmodel.ReceiptOrder{}, &outmodel.ShipmentOrder{}, &stocktakemodel.StocktakeOrder{}, &invmodel.InventoryTrans{}} {
				var own, other int64
				if err := db.WithContext(ctx).Model(model).Count(&own).Error; err != nil || own < 1 {
					t.Fatalf("missing real business rows %T: %d %v", model, own, err)
				}
				if err := db.WithContext(tenant.WithTenant(ctx, 10002)).Model(model).Count(&other).Error; err != nil || other != 0 {
					t.Fatalf("cross-tenant visibility %T: %d %v", model, other, err)
				}
			}
			assertApplicationTenantWrites(ctx, t, db)
		})
	}
}

// Compare every application column name and critical storage types. Historical
// nullable/default/index-name differences are not normalized into false parity.
// Quantity type/boundary tests are additionally exercised in migrations/quantity_test.go.
func schemaColumnContract(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	var rows []struct{ TableName, ColumnName, ColumnType string }
	if err := db.Raw(`SELECT table_name, column_name, column_type FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name <> 'schema_migrations'`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) < 200 {
		t.Fatalf("incomplete schema scan: %d columns", len(rows))
	}
	result := make(map[string]string, len(rows))
	for _, row := range rows {
		typ := "present"
		if row.ColumnName == "tenant_id" || row.TableName == "wms_idempotency" || strings.HasSuffix(row.ColumnName, "_qty") || strings.Contains(row.ColumnName, "quantity") {
			typ = strings.ToLower(row.ColumnType)
		}
		result[row.TableName+"."+row.ColumnName] = typ
	}
	return result
}

func assertTenantUniqueIndexes(t *testing.T, db *gorm.DB) {
	t.Helper()
	var rows []struct{ TableName, Columns string }
	if err := db.Raw(`SELECT table_name, GROUP_CONCAT(column_name ORDER BY seq_in_index) AS columns
		FROM information_schema.statistics WHERE table_schema = DATABASE() AND non_unique = 0
		GROUP BY table_name, index_name`).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	keys := make(map[string]bool)
	for _, row := range rows {
		keys[row.TableName+":"+row.Columns] = true
	}
	for _, key := range []string{
		"wms_warehouse:tenant_id,code", "wms_location:tenant_id,warehouse_id,code",
		"wms_sku:tenant_id,code", "wms_sku:tenant_id,barcode",
		"wms_inventory:tenant_id,warehouse_id,location_id,sku_id,batch_no",
		"wms_receipt_order:tenant_id,order_no", "wms_shipment_order:tenant_id,order_no",
		"wms_shipment_order:tenant_id,biz_order_no", "wms_idempotency:tenant_id,scope,idempotency_key",
	} {
		if !keys[key] {
			t.Errorf("missing tenant unique index %s", key)
		}
	}
}

func assertApplicationTenantWrites(ctx context.Context, t *testing.T, db *gorm.DB) {
	t.Helper()
	row := basicmodel.SKU{Base: modelbase.Base{ID: snowflake.Next()}, Code: "TENANT-CALLBACK", Barcode: "TENANT-CALLBACK", Name: "before"}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil || row.TenantID != 10001 {
		t.Fatalf("create callback: tenant=%d err=%v", row.TenantID, err)
	}
	other := db.WithContext(tenant.WithTenant(ctx, 10002))
	var count int64
	if err := other.Model(&basicmodel.SKU{}).Select("COUNT(*)").Scan(&count).Error; err != nil || count != 0 {
		t.Fatalf("row callback: %d %v", count, err)
	}
	for _, result := range []*gorm.DB{
		other.Model(&basicmodel.SKU{}).Where("id = ?", row.ID).Update("name", "wrong"),
		other.Delete(&basicmodel.SKU{}, row.ID),
	} {
		if result.Error != nil || result.RowsAffected != 0 {
			t.Fatalf("cross-tenant update/delete: rows=%d err=%v", result.RowsAffected, result.Error)
		}
	}
	var unchanged basicmodel.SKU
	if err := db.WithContext(ctx).First(&unchanged, row.ID).Error; err != nil || unchanged.Name != "before" {
		t.Fatalf("tenant write isolation: name=%q err=%v", unchanged.Name, err)
	}
}
