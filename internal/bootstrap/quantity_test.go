package bootstrap

import (
	"os"
	"strings"
	"testing"

	"gowms/internal/pkg/quantity"
	"gowms/internal/testutil"
)

func TestDevelopmentQuantityAuditStopsBeforeNarrowingData(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn)
	if err := db.Exec("CREATE TABLE wms_inventory (stock_quantity BIGINT)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO wms_inventory VALUES (?)", int64(quantity.Max)+1).Error; err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err == nil || !strings.Contains(err.Error(), "quantity audit failed") {
		t.Fatalf("unsafe narrowing accepted: %v", err)
	}
	var value int64
	if err := db.Raw("SELECT stock_quantity FROM wms_inventory").Scan(&value).Error; err != nil || value != int64(quantity.Max)+1 {
		t.Fatalf("audit mutated data: %d %v", value, err)
	}
	var columnType string
	if err := db.Raw("SELECT DATA_TYPE FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'wms_inventory' AND column_name = 'stock_quantity'").Scan(&columnType).Error; err != nil || columnType != "bigint" {
		t.Fatalf("audit altered column: %s %v", columnType, err)
	}
	if err := db.Exec("UPDATE wms_inventory SET stock_quantity = ?", quantity.Max).Error; err != nil {
		t.Fatal(err)
	}
	if err := auditQuantityColumns(db); err != nil {
		t.Fatalf("valid boundary failed audit: %v", err)
	}
}
