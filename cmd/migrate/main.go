// Command migrate applies versioned database migrations and initializes
// the built-in admin or development demo data as separate subcommands.
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
	"gorm.io/gorm"

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
	steps := flag.Int("steps", 1, "number of migrations for down")
	forceVersion := flag.Int("version", 0, "migration version for force")
	flag.Usage = usage
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
	case "bootstrap-admin":
		if err := seedBootstrapAdmin(cfg); err != nil {
			return err
		}
	case "seed-demo":
		if err := seedDemo(cfg); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown command %q: %w", flag.Arg(0), errUsage)
	}
	return nil
}

// seedBootstrapAdmin 首次创建/修复平台管理员：密码取 WMS_ADMIN_PASSWORD，
// release 模式未配置时直接失败，管理员已存在时幂等跳过（不再要求密码）。
func seedBootstrapAdmin(cfg *config.Config) error {
	if err := withDB(cfg, func(db *gorm.DB) error {
		return bootstrap.SeedAdmin(db, cfg)
	}); err != nil {
		return fmt.Errorf("bootstrap admin: %w", err)
	}
	log.Println("bootstrap admin completed")
	return nil
}

// seedDemo 写入开发/演示数据与公开体验账号，仅允许开发/演示环境使用。
func seedDemo(cfg *config.Config) error {
	if cfg.Server.Mode == "release" {
		return errors.New("seed-demo is only allowed in development/demo mode")
	}
	if err := withDB(cfg, func(db *gorm.DB) error {
		return bootstrap.SeedDemo(db, cfg)
	}); err != nil {
		return fmt.Errorf("seed demo: %w", err)
	}
	log.Println("seed demo completed")
	return nil
}

// withDB 按应用配置打开数据库，执行 fn 后关闭连接。
func withDB(cfg *config.Config, fn func(*gorm.DB) error) error {
	db, err := bootstrap.InitDB(cfg)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get sql database: %w", err)
	}
	defer func() {
		if err := sqlDB.Close(); err != nil {
			log.Printf("close database: %v", err)
		}
	}()
	return fn(db)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: migrate [flags] <up|down|version|force|bootstrap-admin|seed-demo>")
	fmt.Fprintln(os.Stderr, "examples:")
	fmt.Fprintln(os.Stderr, "  migrate up                              # 只执行数据库结构迁移")
	fmt.Fprintln(os.Stderr, "  migrate bootstrap-admin                 # 创建/修复平台管理员（release 需要 WMS_ADMIN_PASSWORD）")
	fmt.Fprintln(os.Stderr, "  migrate seed-demo                       # 演示数据与体验账号（release 禁止）")
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
