package testutil

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// OpenIsolatedMySQL creates a temporary schema, migrates only the requested models,
// and drops the schema when the test finishes. It never writes test data into the
// developer's working database.
func OpenIsolatedMySQL(t *testing.T, baseDSN string, models ...any) *gorm.DB {
	t.Helper()

	cfg, err := mysqlDriver.ParseDSN(baseDSN)
	if err != nil {
		t.Fatalf("parse mysql dsn: %v", err)
	}
	adminCfg := *cfg
	adminCfg.DBName = ""
	admin, err := sql.Open("mysql", adminCfg.FormatDSN())
	if err != nil {
		t.Fatalf("open mysql admin connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		_ = admin.Close()
		if os.Getenv("WMS_TEST_REQUIRED") == "1" {
			t.Fatalf("mysql required but unavailable: %v", err)
		}
		t.Skipf("mysql unavailable, skip: %v", err)
	}

	databaseName := fmt.Sprintf("gowms_test_%d_%d", os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + databaseName + "` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = admin.Close()
		t.Fatalf("create test database: %v", err)
	}

	testCfg := *cfg
	testCfg.DBName = databaseName
	db, err := gorm.Open(mysql.Open(testCfg.FormatDSN()), &gorm.Config{
		Logger:         logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		_, _ = admin.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`")
		_ = admin.Close()
		t.Fatalf("open isolated test database: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
		if _, dropErr := admin.Exec("DROP DATABASE IF EXISTS `" + databaseName + "`"); dropErr != nil {
			t.Errorf("drop test database %s: %v", databaseName, dropErr)
		}
		_ = admin.Close()
	})

	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			t.Fatalf("migrate isolated test database: %v", err)
		}
	}
	return db
}

func IsolatedDatabaseName(dsn string) string {
	cfg, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.DBName)
}
