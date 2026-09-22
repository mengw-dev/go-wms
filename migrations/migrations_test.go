package migrations_test

import (
	"os"
	"slices"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	"gowms/internal/testutil"
	"gowms/migrations"
)

func indexColumns(t *testing.T, db *gorm.DB, table, index string) []string {
	t.Helper()
	var columns []string
	err := db.Raw(`SELECT COLUMN_NAME
		FROM information_schema.statistics
		WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?
		ORDER BY seq_in_index`, table, index).Scan(&columns).Error
	if err != nil {
		t.Fatalf("read index %s columns: %v", index, err)
	}
	return columns
}

func TestMigrationsAndImportTokenRollback(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MultiStatements = true
	db := testutil.OpenIsolatedMySQL(t, cfg.FormatDSN())
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
		sourceErr, dbErr := m.Close()
		if sourceErr != nil || dbErr != nil {
			t.Errorf("close migrations: %v, %v", sourceErr, dbErr)
		}
	})
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn("wms_import_task", "run_token") {
		t.Fatal("missing run_token")
	}
	if !db.Migrator().HasIndex("wms_inventory", "idx_inv_tenant_fifo") {
		t.Fatal("missing tenant-aware FIFO index")
	}
	if got := indexColumns(t, db, "wms_inventory", "idx_inv_tenant_fifo"); !slices.Equal(got, []string{
		"tenant_id", "warehouse_id", "sku_id", "available_quantity", "stock_in_time",
	}) {
		t.Fatalf("tenant FIFO index columns=%v", got)
	}
	if !db.Migrator().HasIndex("wms_location", "uk_loc_wh_code") {
		t.Fatal("missing tenant-aware location unique index")
	}
	if got := indexColumns(t, db, "wms_location", "uk_loc_wh_code"); !slices.Equal(got, []string{
		"tenant_id", "warehouse_id", "code",
	}) {
		t.Fatalf("location unique index columns=%v", got)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn("wms_import_task", "run_token") {
		t.Fatal("rollback of index migration removed run_token")
	}
	if db.Migrator().HasIndex("wms_inventory", "idx_inv_tenant_fifo") {
		t.Fatal("index migration rollback retained tenant-aware FIFO index")
	}
	if !db.Migrator().HasIndex("wms_inventory", "idx_inv_fifo") {
		t.Fatal("index migration rollback did not restore legacy FIFO index")
	}
	if got := indexColumns(t, db, "wms_inventory", "idx_inv_fifo"); !slices.Equal(got, []string{
		"warehouse_id", "sku_id", "available_quantity", "stock_in_time",
	}) {
		t.Fatalf("legacy FIFO index columns=%v", got)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn("wms_import_task", "run_token") {
		t.Fatal("rollback retained run_token")
	}
	if err := m.Steps(2); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasColumn("wms_import_task", "run_token") {
		t.Fatal("reapply missing run_token")
	}
	if !db.Migrator().HasIndex("wms_inventory", "idx_inv_tenant_fifo") {
		t.Fatal("reapply missing tenant-aware FIFO index")
	}
	if got := indexColumns(t, db, "wms_inventory", "idx_inv_tenant_fifo"); !slices.Equal(got, []string{
		"tenant_id", "warehouse_id", "sku_id", "available_quantity", "stock_in_time",
	}) {
		t.Fatalf("reapplied tenant FIFO index columns=%v", got)
	}
}
