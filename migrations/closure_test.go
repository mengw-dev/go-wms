package migrations_test

import (
	"os"
	"testing"

	"gowms/internal/bootstrap"
	"gowms/internal/testutil"
	"gowms/migrations"
)

func TestClosureSchemaAndFreezeRollbackProtectAudit(t *testing.T) {
	db := testutil.OpenMigratedMySQL(t, os.Getenv("WMS_TEST_DSN"))
	if e := bootstrap.VerifyQuantityChecks(db); e != nil {
		t.Fatal(e)
	}
	if e := db.Exec(`INSERT INTO wms_inventory(id,warehouse_id,location_id,sku_id,stock_quantity,available_quantity,allocated_quantity,frozen_quantity) VALUES (1,1,1,1,10,5,2,3)`).Error; e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{
		"UPDATE wms_inventory SET frozen_quantity=-1 WHERE id=1",
		"UPDATE wms_inventory SET available_quantity=6 WHERE id=1",
	} {
		if e := db.Exec(query).Error; e == nil {
			t.Fatalf("invalid quantity accepted: %s", query)
		}
	}
	raw, e := migrations.FS.ReadFile("versions/000016_inventory_freeze.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	pool, e := db.DB()
	if e != nil {
		t.Fatal(e)
	}
	conn, e := pool.Conn(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer conn.Close()
	for _, statement := range splitWaveSQL(string(raw)) {
		if _, e = conn.ExecContext(t.Context(), statement); e != nil {
			break
		}
	}
	if e == nil || !db.Migrator().HasColumn("wms_inventory", "frozen_quantity") {
		t.Fatalf("rollback erased frozen stock: %v", e)
	}
	var frozen int
	if e = db.Raw("SELECT frozen_quantity FROM wms_inventory WHERE id=1").Scan(&frozen).Error; e != nil || frozen != 3 {
		t.Fatalf("stock changed=%d %v", frozen, e)
	}
	if e = db.Exec("UPDATE wms_inventory SET frozen_quantity=0,available_quantity=8 WHERE id=1").Error; e != nil {
		t.Fatal(e)
	}
	if e = db.Exec("INSERT INTO wms_inventory_trans(id,inventory_id,trans_type,quantity_change,before_quantity,after_quantity,available_before,available_after,frozen_before,frozen_after) VALUES (1,1,'FREEZE',0,10,10,8,5,0,3)").Error; e != nil {
		t.Fatal(e)
	}
	for _, statement := range splitWaveSQL(string(raw)) {
		if _, e = conn.ExecContext(t.Context(), statement); e != nil {
			break
		}
	}
	if e == nil || !db.Migrator().HasColumn("wms_inventory_trans", "frozen_after") {
		t.Fatalf("rollback erased freeze audit: %v", e)
	}
}
