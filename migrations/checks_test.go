package migrations_test

import (
	"errors"
	"os"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	"gowms/internal/bootstrap"
	"gowms/internal/testutil"
	"gowms/migrations"
)

func checkMigrator(t *testing.T) (*gorm.DB, *migrate.Migrate) {
	t.Helper()
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
	t.Cleanup(func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			t.Errorf("close migrations: %v %v", sourceErr, dbErr)
		}
	})
	return db, m
}

var progressChecks = map[string]string{
	"wms_task":           "chk_task_quantity_progress",
	"wms_allocation":     "chk_allocation_quantity_progress",
	"wms_shipment_order": "chk_shipment_quantity_progress",
}

func TestProgressChecksMatchBothCreationPaths(t *testing.T) {
	var clauses map[string]string
	for _, formal := range []bool{false, true} {
		name := "AutoMigrate"
		if formal {
			name = "versioned SQL"
		}
		t.Run(name, func(t *testing.T) {
			db, m := checkMigrator(t)
			if formal {
				if err := m.Migrate(12); err != nil {
					t.Fatal(err)
				}
			} else if err := bootstrap.AutoMigrate(db); err != nil {
				t.Fatal(err)
			}
			got := make(map[string]string)
			for table, name := range progressChecks {
				var metadata struct{ CheckClause, Enforced string }
				err := db.Raw(`SELECT cc.CHECK_CLAUSE, tc.ENFORCED FROM information_schema.table_constraints tc
					JOIN information_schema.check_constraints cc ON cc.constraint_schema = tc.constraint_schema AND cc.constraint_name = tc.constraint_name
					WHERE tc.table_schema = DATABASE() AND tc.table_name = ? AND tc.constraint_name = ?`, table, name).Scan(&metadata).Error
				if err != nil || metadata.CheckClause == "" || metadata.Enforced != "YES" {
					t.Fatalf("missing or disabled %s: %+v %v", name, metadata, err)
				}
				got[table] = metadata.CheckClause
				if formal && clauses[table] != metadata.CheckClause {
					t.Fatalf("creation paths differ for %s: %q / %q", table, clauses[table], metadata.CheckClause)
				}
			}
			clauses = got
			assertProgressWrites(t, db)
		})
	}
}

func assertProgressWrites(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, query := range []string{
		"INSERT INTO wms_task (id,task_no,task_type,order_id,sku_id,warehouse_id,target_qty,done_qty) VALUES (1,'T','PICK',1,1,1,1,0)",
		"INSERT INTO wms_allocation (id,order_id,detail_id,inventory_id,sku_id,location_id,allocated_qty,picked_qty) VALUES (1,1,1,1,1,1,1,0)",
		"INSERT INTO wms_shipment_order (id,order_no,biz_order_no,warehouse_id,expected_qty,allocated_qty,picked_qty) VALUES (1,'S','B',1,1,1,0)",
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatal(err)
		}
	}
	for table, assignments := range map[string][]string{
		"wms_task":           {"target_qty=-1", "done_qty=-1", "done_qty=2"},
		"wms_allocation":     {"allocated_qty=-1", "picked_qty=-1", "picked_qty=2"},
		"wms_shipment_order": {"expected_qty=-1", "allocated_qty=-1", "picked_qty=-1", "picked_qty=2", "allocated_qty=2"},
	} {
		for _, assignment := range assignments {
			err := db.Exec("UPDATE " + table + " SET " + assignment + " WHERE id=1").Error
			var driverErr *mysqldriver.MySQLError
			if !errors.As(err, &driverErr) || driverErr.Number != 3819 {
				t.Fatalf("%s %s not rejected by CHECK: %v", table, assignment, err)
			}
		}
	}
	for _, query := range []string{
		"UPDATE wms_task SET target_qty=2147483647,done_qty=2147483647 WHERE id=1",
		"UPDATE wms_allocation SET allocated_qty=2147483647,picked_qty=2147483647 WHERE id=1",
		"UPDATE wms_shipment_order SET expected_qty=2147483647,allocated_qty=2147483647,picked_qty=2147483647 WHERE id=1",
		"UPDATE wms_task SET target_qty=0,done_qty=0 WHERE id=1",
		"UPDATE wms_allocation SET allocated_qty=0,picked_qty=0 WHERE id=1",
		"UPDATE wms_shipment_order SET expected_qty=0,allocated_qty=0,picked_qty=0 WHERE id=1",
	} {
		if err := db.Exec(query).Error; err != nil {
			t.Fatalf("valid zero/max boundary rejected: %v", err)
		}
	}
}

func TestProgressCheckMigrationRollbackAndReapply(t *testing.T) {
	db, m := checkMigrator(t)
	if err := m.Migrate(12); err != nil {
		t.Fatal(err)
	}
	assertProgressWrites(t, db)
	if err := m.Migrate(11); err != nil {
		t.Fatal(err)
	}
	for table, name := range progressChecks {
		if hasCheckConstraint(t, db, table, name) {
			t.Fatalf("rollback retained %s", name)
		}
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM " + table).Scan(&count).Error; err != nil || count != 1 {
			t.Fatalf("rollback changed data in %s: %d %v", table, count, err)
		}
	}
	if !hasCheckConstraint(t, db, "wms_inventory", "chk_inv_quantity_balance") {
		t.Fatal("rollback removed existing inventory constraint")
	}
	if err := m.Migrate(12); err != nil {
		t.Fatal(err)
	}
	if err := bootstrap.AuditQuantityChecks(db); err != nil {
		t.Fatal(err)
	}
}
