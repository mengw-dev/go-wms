package bootstrap

import (
	"errors"
	"os"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/testutil"
)

func checkDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.OpenIsolatedMySQL(t, os.Getenv("WMS_TEST_DSN"))
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestQuantityChecksAreVerifiedAndRepeatable(t *testing.T) {
	db := checkDatabase(t)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	for _, check := range quantityChecks {
		if exists, err := checkDefinition(db, check); err != nil || !exists {
			t.Fatalf("%s: exists=%t err=%v", check.name, exists, err)
		}
	}
}

func TestReleaseCheckVerificationIsReadOnlyAndRequiresEveryConstraint(t *testing.T) {
	db := checkDatabase(t)
	if err := VerifyQuantityChecks(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE wms_task DROP CHECK chk_task_quantity_progress").Error; err != nil {
		t.Fatal(err)
	}
	if err := VerifyQuantityChecks(db); err == nil || !strings.Contains(err.Error(), "missing check") {
		t.Fatalf("release accepted incomplete database: %v", err)
	}
	if exists, err := checkDefinition(db, quantityCheck{"wms_task", "chk_task_quantity_progress", "(target_qty >= 0) AND (done_qty >= 0) AND (done_qty <= target_qty)", []string{"target_qty", "done_qty"}}); exists || err != nil {
		t.Fatalf("verification altered schema: exists=%t err=%v", exists, err)
	}
}

func TestQuantityCheckDDLStillRejectsDataChangedAfterAudit(t *testing.T) {
	db := checkDatabase(t)
	if err := db.Exec("ALTER TABLE wms_inventory DROP CHECK chk_inv_allocated_non_negative").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	const callback = "test:write_after_audit"
	inserted := false
	if err := db.Callback().Raw().Before("gorm:raw").Register(callback, func(q *gorm.DB) {
		if !inserted && strings.Contains(q.Statement.SQL.String(), "ADD CONSTRAINT `chk_inv_allocated_non_negative`") {
			inserted = true
			_, err := sqlDB.Exec(`INSERT INTO wms_inventory
				(id,warehouse_id,location_id,sku_id,stock_quantity,available_quantity,allocated_quantity)
				VALUES (1,1,1,1,1,2,-1)`)
			if err != nil {
				_ = q.AddError(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Raw().Remove(callback) })
	err = AutoMigrate(db)
	var driverErr *mysqldriver.MySQLError
	if !errors.As(err, &driverErr) || driverErr.Number != 3819 || !inserted {
		t.Fatalf("DDL violation ignored: inserted=%t err=%v", inserted, err)
	}
	var allocated int
	if err := db.Raw("SELECT allocated_quantity FROM wms_inventory WHERE id=1").Scan(&allocated).Error; err != nil || allocated != -1 {
		t.Fatalf("failed DDL changed existing data: %d %v", allocated, err)
	}
}

func TestQuantityChecksRejectWrongDefinitionAndDisabledConstraint(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong definition", true: "not enforced"}[disabled], func(t *testing.T) {
			db := checkDatabase(t)
			query := "ALTER TABLE wms_inventory ALTER CHECK chk_inv_allocated_non_negative NOT ENFORCED"
			if !disabled {
				if err := db.Exec("ALTER TABLE wms_inventory DROP CHECK chk_inv_allocated_non_negative").Error; err != nil {
					t.Fatal(err)
				}
				query = "ALTER TABLE wms_inventory ADD CONSTRAINT chk_inv_allocated_non_negative CHECK (allocated_quantity >= -1)"
			}
			if err := db.Exec(query).Error; err != nil {
				t.Fatal(err)
			}
			if err := AutoMigrate(db); err == nil || !strings.Contains(err.Error(), "definition mismatch") {
				t.Fatalf("invalid existing CHECK accepted: %v", err)
			}
		})
	}
}

func TestQuantityCheckDDLFailureRetainsDriverError(t *testing.T) {
	db := checkDatabase(t)
	if err := db.Exec("ALTER TABLE wms_inventory DROP CHECK chk_inv_allocated_non_negative").Error; err != nil {
		t.Fatal(err)
	}
	const callback = "test:deny_check_ddl"
	if err := db.Callback().Raw().Before("gorm:raw").Register(callback, func(q *gorm.DB) {
		if strings.Contains(q.Statement.SQL.String(), "ADD CONSTRAINT `chk_inv_allocated_non_negative`") {
			_ = q.AddError(&mysqldriver.MySQLError{Number: 1142, Message: "ALTER denied"})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Raw().Remove(callback) })
	err := AutoMigrate(db)
	var driverErr *mysqldriver.MySQLError
	if !errors.As(err, &driverErr) || driverErr.Number != 1142 {
		t.Fatalf("DDL failure was swallowed: %v", err)
	}
}

func TestQuantityCheckDuplicateMustBelongToCorrectTable(t *testing.T) {
	db := checkDatabase(t)
	if err := db.Exec("ALTER TABLE wms_inventory DROP CHECK chk_inv_allocated_non_negative").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE unrelated (n INT, CONSTRAINT chk_inv_allocated_non_negative CHECK (n >= 0))").Error; err != nil {
		t.Fatal(err)
	}
	err := AutoMigrate(db)
	var driverErr *mysqldriver.MySQLError
	if !errors.As(err, &driverErr) || driverErr.Number != 3822 {
		t.Fatalf("wrong-table duplicate accepted, err=%v", err)
	}
}

func TestQuantityCheckConcurrentCorrectDDLIsVerified(t *testing.T) {
	db := checkDatabase(t)
	if err := db.Exec("ALTER TABLE wms_inventory DROP CHECK chk_inv_allocated_non_negative").Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	const callback = "test:concurrent_check"
	created := false
	if err := db.Callback().Raw().Before("gorm:raw").Register(callback, func(q *gorm.DB) {
		if !created && strings.Contains(q.Statement.SQL.String(), "ADD CONSTRAINT `chk_inv_allocated_non_negative`") {
			created = true
			if _, err := sqlDB.Exec("ALTER TABLE wms_inventory ADD CONSTRAINT chk_inv_allocated_non_negative CHECK (allocated_quantity >= 0)"); err != nil {
				_ = q.AddError(err)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Raw().Remove(callback) })
	if err := AutoMigrate(db); err != nil || !created {
		t.Fatalf("correct concurrent DDL not verified: created=%t err=%v", created, err)
	}
}

func TestNormalizeCheckKeepsArithmeticPrecedence(t *testing.T) {
	if normalizeCheck(" ((`a` = (`b` + `c`))) ") != normalizeCheck("a = (b + c)") {
		t.Fatal("harmless formatting not normalized")
	}
	if normalizeCheck("a = (b + c)") == normalizeCheck("(a = b) + c") {
		t.Fatal("normalization erased arithmetic precedence")
	}
}
