package migrations_test

import (
	"os"
	"strings"
	"testing"

	outmodel "gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/testutil"
	"gowms/migrations"
)

func TestWaveMigrationRollbackRequiresNoHistory(t *testing.T) {
	for _, populated := range []bool{false, true} {
		name := "empty"
		if populated {
			name = "history"
		}
		t.Run(name, func(t *testing.T) {
			dsn := os.Getenv("WMS_TEST_DSN")
			if dsn == "" {
				dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
			}
			db := testutil.OpenMigratedMySQL(t, dsn)
			if !hasCheckConstraint(t, db, "wms_wave_line", "chk_wave_line_qty") || !hasCheckConstraint(t, db, "wms_wave_item", "chk_wave_item_qty") {
				t.Fatal("missing wave checks")
			}
			if populated {
				if err := db.Create(&outmodel.Wave{Base: modelbase.Base{ID: 1}, WaveNo: "HISTORY", WarehouseID: 1}).Error; err != nil {
					t.Fatal(err)
				}
			}
			raw, err := migrations.FS.ReadFile("versions/000015_wave_picking.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			// Match migrate's multi-statement execution on one connection. Empty
			// rollback is also exercised by the existing migrate up/down suite.
			pool, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			conn, err := pool.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			// Run statements separately because the fixture DSN need not enable
			// multiStatements, and stop at the first safety check failure.
			for _, statement := range splitWaveSQL(string(raw)) {
				_, err = conn.ExecContext(t.Context(), statement)
				if err != nil {
					break
				}
			}
			if populated {
				if err == nil || !db.Migrator().HasTable("wms_wave") || !db.Migrator().HasColumn("wms_task", "wave_id") {
					t.Fatalf("history guard: %v", err)
				}
			} else if err != nil || db.Migrator().HasTable("wms_wave") || db.Migrator().HasColumn("wms_task", "wave_id") {
				t.Fatalf("empty rollback: %v", err)
			}
		})
	}
}

func splitWaveSQL(raw string) []string {
	var result []string
	for _, statement := range strings.Split(raw, ";") {
		if strings.TrimSpace(statement) != "" {
			result = append(result, statement)
		}
	}
	return result
}
