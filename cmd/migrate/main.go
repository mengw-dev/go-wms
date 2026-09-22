// Command migrate applies versioned database migrations and optional seed data.
package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"gowms/internal/bootstrap"
	"gowms/internal/pkg/config"
	"gowms/migrations"
)

var errUsage = errors.New("invalid command usage")

func main() {
	if err := run(); err != nil {
		if errors.Is(err, errUsage) {
			usage()
			os.Exit(2)
		}
		log.Fatal(err)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	seed := flag.Bool("seed", false, "seed initial admin after up")
	steps := flag.Int("steps", 1, "number of migrations for down")
	forceVersion := flag.Int("version", 0, "migration version for force")
	flag.Parse()

	if flag.NArg() != 1 {
		return errUsage
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	sqlDB, err := sql.Open("mysql", migrationDSN(cfg.MySQL.DSN))
	if err != nil {
		return fmt.Errorf("open mysql: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("close mysql: %v", err)
		}
	}()
	if err := waitForMySQL(sqlDB, 90*time.Second); err != nil {
		return fmt.Errorf("ping mysql: %w", err)
	}

	driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
	if err != nil {
		return fmt.Errorf("create migration driver: %w", err)
	}
	source, err := iofs.New(migrations.FS, "versions")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}

	switch flag.Arg(0) {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migrate up: %w", err)
		}
		if *seed {
			if err := seedData(cfg); err != nil {
				return err
			}
		}
		log.Println("migration up completed")
	case "down":
		if *steps <= 0 {
			return errors.New("steps must be positive")
		}
		if err := m.Steps(-*steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("migrate down: %w", err)
		}
		log.Printf("migration down completed: %d steps", *steps)
	case "force":
		if *forceVersion < 0 {
			return errors.New("version must be non-negative")
		}
		if err := m.Force(*forceVersion); err != nil {
			return fmt.Errorf("migrate force: %w", err)
		}
		log.Printf("migration version forced to %d", *forceVersion)
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			return fmt.Errorf("migration version: %w", err)
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
	default:
		return fmt.Errorf("unknown command %q: %w", flag.Arg(0), errUsage)
	}
	return nil
}

func seedData(cfg *config.Config) error {
	db, err := bootstrap.InitDB(cfg)
	if err != nil {
		return fmt.Errorf("init seed database: %w", err)
	}
	sqlSeedDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get seed sql db: %w", err)
	}
	defer func() {
		if err := sqlSeedDB.Close(); err != nil {
			log.Printf("close seed database: %v", err)
		}
	}()
	if err := bootstrap.Seed(db); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	if err := bootstrap.SeedDemoAccounts(db, cfg); err != nil {
		return fmt.Errorf("seed demo accounts: %w", err)
	}
	if err := bootstrap.SeedPersonalAccounts(db, cfg); err != nil {
		return fmt.Errorf("seed personal accounts: %w", err)
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate [flags] <up|down|version|force>")
	fmt.Fprintln(os.Stderr, "examples:")
	fmt.Fprintln(os.Stderr, "  migrate -seed up")
	fmt.Fprintln(os.Stderr, "  migrate -steps 1 down")
}

func waitForMySQL(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		err := db.Ping()
		if err == nil {
			return nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(2 * time.Second)
	}
}

func migrationDSN(dsn string) string {
	if strings.Contains(dsn, "multiStatements=") {
		return dsn
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "multiStatements=true"
}
