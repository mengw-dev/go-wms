package repository

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func newDryRunDB(t *testing.T, output *bytes.Buffer) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: "test@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true,
		Logger: gormlogger.New(log.New(output, "", 0), gormlogger.Config{LogLevel: gormlogger.Info}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUpdateWarehouseStatusDoesNotTouchOtherFields(t *testing.T) {
	var output bytes.Buffer
	db := newDryRunDB(t, &output)
	if err := New().UpdateWarehouseStatus(context.Background(), db, 42, 0); err != nil {
		t.Fatal(err)
	}
	sql := output.String()

	if !strings.Contains(sql, "UPDATE `wms_warehouse`") || !strings.Contains(sql, "status") {
		t.Fatalf("status update SQL = %q", sql)
	}
	if strings.Contains(sql, "name") || strings.Contains(sql, "remark") {
		t.Fatalf("status update touched unrelated fields: %q", sql)
	}
}

func TestListWarehousesAppliesStatusFilter(t *testing.T) {
	var output bytes.Buffer
	db := newDryRunDB(t, &output)
	status := 0
	if _, _, err := New().ListWarehouses(context.Background(), db, "", &status, 1, 10); err != nil {
		t.Fatal(err)
	}
	if sql := output.String(); !strings.Contains(sql, "status = 0") {
		t.Fatalf("warehouse status filter missing: %q", sql)
	}
}

func TestListLocationsAppliesZoneStatusAndKeywordFilters(t *testing.T) {
	var output bytes.Buffer
	db := newDryRunDB(t, &output)
	status := 2
	if _, _, err := New().ListLocations(context.Background(), db, 7, "A01", &status, "A01", 1, 10); err != nil {
		t.Fatal(err)
	}
	sql := output.String()
	if !strings.Contains(sql, "zone = 'A01'") || !strings.Contains(sql, "status = 2") || !strings.Contains(sql, "code LIKE") {
		t.Fatalf("location filters missing: %q", sql)
	}
}
