package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"gowms/internal/bootstrap"
	"gowms/internal/pkg/config"
	"gowms/migrations"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	seed := flag.Bool("seed", false, "seed initial admin after up")
	steps := flag.Int("steps", 1, "number of migrations for down")
	forceVersion := flag.Int("version", 0, "migration version for force")
	flag.Parse()

	if flag.NArg() != 1 {
		usage()
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	sqlDB, err := sql.Open("mysql", migrationDSN(cfg.MySQL.DSN))
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("ping mysql: %v", err)
	}
	defer sqlDB.Close()

	driver, err := migratemysql.WithInstance(sqlDB, &migratemysql.Config{})
	if err != nil {
		log.Fatalf("create migration driver: %v", err)
	}
	source, err := iofs.New(migrations.FS, "versions")
	if err != nil {
		log.Fatalf("open embedded migrations: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", source, "mysql", driver)
	if err != nil {
		log.Fatalf("init migrator: %v", err)
	}

	switch flag.Arg(0) {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("migrate up: %v", err)
		}
		if *seed {
			db, err := bootstrap.InitDB(cfg)
			if err != nil {
				log.Fatalf("init seed database: %v", err)
			}
			sqlSeedDB, err := db.DB()
			if err != nil {
				log.Fatalf("get seed sql db: %v", err)
			}
			defer sqlSeedDB.Close()
			if err := bootstrap.Seed(db); err != nil {
				log.Fatalf("seed: %v", err)
			}
		}
		log.Println("migration up completed")
	case "down":
		if *steps <= 0 {
			log.Fatal("steps must be positive")
		}
		if err := m.Steps(-*steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			log.Fatalf("migrate down: %v", err)
		}
		log.Printf("migration down completed: %d steps", *steps)
	case "force":
		if *forceVersion < 0 {
			log.Fatal("version must be non-negative")
		}
		if err := m.Force(*forceVersion); err != nil {
			log.Fatalf("migrate force: %v", err)
		}
		log.Printf("migration version forced to %d", *forceVersion)
	case "version":
		version, dirty, err := m.Version()
		if err != nil {
			log.Fatalf("migration version: %v", err)
		}
		fmt.Printf("version=%d dirty=%t\n", version, dirty)
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate [flags] <up|down|version|force>")
	fmt.Fprintln(os.Stderr, "examples:")
	fmt.Fprintln(os.Stderr, "  migrate -seed up")
	fmt.Fprintln(os.Stderr, "  migrate -steps 1 down")
	os.Exit(2)
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
