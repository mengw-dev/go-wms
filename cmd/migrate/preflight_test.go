package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"gowms/internal/testutil"
	"gowms/migrations"
)

// 在子进程中运行真实 CLI，避免 flag/config 全局状态污染其他测试。
func TestMigrationCommandHelper(t *testing.T) {
	t.Helper()
	if os.Getenv("WMS_MIGRATION_COMMAND_TEST") != "1" {
		return
	}
	flag.CommandLine = flag.NewFlagSet("migrate", flag.ContinueOnError)
	os.Args = []string{"migrate", "-config", os.Getenv("WMS_MIGRATION_CONFIG_TEST"), "up"}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestMigrationPreflightPreservesDirtyHistoricalDataAndVersion(t *testing.T) {
	cfg, err := mysqldriver.ParseDSN(os.Getenv("WMS_TEST_DSN"))
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
	t.Cleanup(func() { _, _ = m.Close() })
	if err := m.Migrate(11); err != nil {
		t.Fatal(err)
	}
	// 覆盖不同租户与软删行；历史演示数据 picked>allocated，不能自动修账或跳过。
	if err := db.Exec(`INSERT INTO wms_shipment_order
		(id,tenant_id,order_no,biz_order_no,warehouse_id,expected_qty,allocated_qty,picked_qty,deleted_at)
		VALUES (1,77,'HISTORICAL','HISTORICAL',1,30,0,30,NOW())`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT DATABASE()").Scan(&cfg.DBName).Error; err != nil {
		t.Fatal(err)
	}
	configPath, err := filepath.Abs("../../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	invoke := func() (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestMigrationCommandHelper$") // #nosec G204 -- rerun this test executable, no shell.
		cmd.Env = append(os.Environ(), "WMS_MIGRATION_COMMAND_TEST=1", "WMS_MIGRATION_CONFIG_TEST="+configPath,
			"WMS_MYSQL_DSN="+cfg.FormatDSN(), "WMS_SERVER_MODE=debug")
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	output, err := invoke()
	if err == nil || !strings.Contains(output, "quantity check audit failed") {
		t.Fatalf("historical invalid data accepted: %v %s", err, output)
	}
	version, dirty, err := m.Version()
	if err != nil || dirty || version != 11 {
		t.Fatalf("preflight changed migration state: %d %t %v", version, dirty, err)
	}
	var picked, allocated, checks int
	if err := db.Raw("SELECT picked_qty, allocated_qty FROM wms_shipment_order WHERE id=1").Row().Scan(&picked, &allocated); err != nil {
		t.Fatal(err)
	}
	if picked != 30 || allocated != 0 {
		t.Fatalf("historical data silently changed: picked=%d allocated=%d", picked, allocated)
	}
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.table_constraints
		WHERE constraint_schema=DATABASE() AND constraint_name IN ('chk_task_quantity_progress','chk_allocation_quantity_progress','chk_shipment_quantity_progress')`).Scan(&checks).Error; err != nil || checks != 0 {
		t.Fatalf("business DDL ran before preflight: %d %v", checks, err)
	}
	// 只修复本测试创建的 fixture，再执行 CLI，证明正常升级与审计拒绝两条路径。
	if err := db.Exec("UPDATE wms_shipment_order SET allocated_qty=30 WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	if output, err := invoke(); err != nil {
		t.Fatalf("valid CLI migration failed: %v %s", err, output)
	}
	version, dirty, err = m.Version()
	if err != nil || dirty || version < 12 {
		t.Fatalf("valid migration not completed: %d %t %v", version, dirty, err)
	}
}
