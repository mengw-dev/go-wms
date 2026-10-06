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

func hasCheckConstraint(t *testing.T, db *gorm.DB, table, constraint string) bool {
	t.Helper()
	var count int64
	err := db.Raw(`SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE constraint_schema = DATABASE() AND table_name = ? AND constraint_name = ?
		AND constraint_type = 'CHECK'`, table, constraint).Scan(&count).Error
	if err != nil {
		t.Fatalf("read check constraint %s: %v", constraint, err)
	}
	return count > 0
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
	if !db.Migrator().HasTable("wms_idempotency") {
		t.Fatal("missing idempotency table")
	}
	if got := indexColumns(t, db, "wms_idempotency", "uk_idem_tenant_scope_key"); !slices.Equal(got, []string{
		"tenant_id", "scope", "idempotency_key",
	}) {
		t.Fatalf("idempotency unique index columns=%v", got)
	}
	if !db.Migrator().HasIndex("wms_idempotency", "idx_idem_created_at") {
		t.Fatal("missing idempotency created_at index")
	}
	for _, column := range []string{"claimed_by", "claim_token", "lease_expire_at"} {
		if !db.Migrator().HasColumn("wms_task", column) {
			t.Fatalf("missing task lease column %s", column)
		}
	}
	for _, constraint := range []string{"chk_inv_allocated_non_negative", "chk_inv_quantity_balance"} {
		if !hasCheckConstraint(t, db, "wms_inventory", constraint) {
			t.Fatalf("missing inventory check constraint %s", constraint)
		}
	}
	if err := m.Migrate(7); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasColumn("wms_task", "claim_token") {
		t.Fatal("lease migration rollback retained claim_token")
	}
	for _, constraint := range []string{"chk_inv_allocated_non_negative", "chk_inv_quantity_balance"} {
		if hasCheckConstraint(t, db, "wms_inventory", constraint) {
			t.Fatalf("inventory invariant rollback retained %s", constraint)
		}
	}
	if !db.Migrator().HasColumn("wms_import_task", "run_token") {
		t.Fatal("inventory invariant rollback removed run_token")
	}
	if !db.Migrator().HasIndex("wms_inventory", "idx_inv_tenant_fifo") {
		t.Fatal("inventory invariant rollback removed tenant-aware FIFO index")
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
	if err := m.Steps(5); err != nil { // 回到版本 10，单独验证 000011 的数据回填
		t.Fatal(err)
	}
	// 构造一条历史拣货任务（detail_id 缺失）与对应分配行，验证 000011 回填
	if err := db.Exec(`INSERT INTO wms_allocation
		(id, tenant_id, order_id, detail_id, inventory_id, sku_id, location_id, allocated_qty, picked_qty, status, version)
		VALUES (900001, 1, 900002, 900003, 900004, 900005, 900006, 2, 0, 'ALLOCATED', 1)`).Error; err != nil {
		t.Fatalf("seed allocation: %v", err)
	}
	if err := db.Exec(`INSERT INTO wms_task
		(id, tenant_id, task_no, task_type, status, order_id, allocation_id, sku_id, warehouse_id, target_qty, done_qty, version)
		VALUES (900007, 1, 'PK-BACKFILL', 'PICK', 'CREATED', 900002, 900001, 900005, 900008, 2, 0, 1)`).Error; err != nil {
		t.Fatalf("seed pick task: %v", err)
	}
	if err := m.Steps(1); err != nil { // 000011：回填 detail_id
		t.Fatal(err)
	}
	var backfilled int64
	if err := db.Raw(`SELECT detail_id FROM wms_task WHERE id = 900007`).Scan(&backfilled).Error; err != nil {
		t.Fatal(err)
	}
	if backfilled != 900003 {
		t.Fatalf("000011 backfill detail_id=%d want 900003", backfilled)
	}
	if !db.Migrator().HasColumn("wms_import_task", "run_token") {
		t.Fatal("reapply missing run_token")
	}
	if !db.Migrator().HasTable("wms_idempotency") {
		t.Fatal("reapply missing idempotency table")
	}
	if !db.Migrator().HasColumn("wms_task", "claim_token") {
		t.Fatal("reapply missing task lease column")
	}
	if !db.Migrator().HasIndex("wms_inventory", "idx_inv_tenant_fifo") {
		t.Fatal("reapply missing tenant-aware FIFO index")
	}
	if got := indexColumns(t, db, "wms_inventory", "idx_inv_tenant_fifo"); !slices.Equal(got, []string{
		"tenant_id", "warehouse_id", "sku_id", "available_quantity", "stock_in_time",
	}) {
		t.Fatalf("reapplied tenant FIFO index columns=%v", got)
	}
	for _, constraint := range []string{"chk_inv_allocated_non_negative", "chk_inv_quantity_balance"} {
		if !hasCheckConstraint(t, db, "wms_inventory", constraint) {
			t.Fatalf("reapply missing inventory check constraint %s", constraint)
		}
	}
}
