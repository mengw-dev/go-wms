package testutil

import (
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

func TestParseTestDSNForcesTimeParsing(t *testing.T) {
	cfg := parseTestDSN(t, "root:pass@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4")
	if !cfg.ParseTime {
		t.Fatal("test DSN must force parseTime=true")
	}
	if cfg.DBName != "gowms" {
		t.Fatalf("database name = %q, want gowms", cfg.DBName)
	}

	roundTrip, err := mysqlDriver.ParseDSN(cfg.FormatDSN())
	if err != nil {
		t.Fatalf("round-trip test DSN: %v", err)
	}
	if !roundTrip.ParseTime {
		t.Fatal("formatted test DSN lost parseTime=true")
	}
}
