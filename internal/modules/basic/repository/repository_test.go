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
