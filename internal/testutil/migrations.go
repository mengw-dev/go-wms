package testutil

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	"gowms/migrations"
)

// OpenMigratedMySQL uses the embedded release migrations, not model AutoMigrate.
// Tenant callbacks and schema ownership are the same as OpenIsolatedMySQL.
func OpenMigratedMySQL(t *testing.T, baseDSN string) *gorm.DB {
	t.Helper()
	cfg := parseTestDSN(t, baseDSN)
	cfg.MultiStatements = true
	db := OpenIsolatedMySQL(t, cfg.FormatDSN())
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
		if srcErr, dbErr := m.Close(); srcErr != nil || dbErr != nil {
			t.Errorf("close test migrations: %v %v", srcErr, dbErr)
		}
	})
	if err := m.Up(); err != nil {
		t.Fatalf("apply release migrations: %v", err)
	}
	return db
}
