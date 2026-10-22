package migrations_test

import (
	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/testutil"
	"gowms/migrations"
	"os"
	"testing"
	"time"
)

func TestMovementReleaseMigrationProtectsReceipts(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenMigratedMySQL(t, dsn)
	if !db.Migrator().HasIndex(&invmodel.Movement{}, "uk_move_operation") || !hasCheckConstraint(t, db, "wms_inventory_movement", "chk_move_positive") {
		t.Fatal("missing movement constraints")
	}
	row := &invmodel.Movement{OperationKey: "durable", RequestHash: "test", MoveNo: "YK1", Quantity: 1, StockInTime: time.Now()}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := *row
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("duplicate movement operation key accepted")
	}
	invalid := *row
	invalid.ID = 0
	invalid.OperationKey = "invalid"
	invalid.Quantity = 0
	if err := db.Create(&invalid).Error; err == nil {
		t.Fatal("non-positive movement quantity accepted")
	}
	down, err := migrations.FS.ReadFile("versions/000014_inventory_movements.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(down)).Error; err == nil {
		t.Fatal("populated movement table silently dropped")
	}
	var count int64
	if err = db.Model(&invmodel.Movement{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("receipt lost: count=%d err=%v", count, err)
	}
}
