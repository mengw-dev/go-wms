// Command wms 启动 HTTP 服务并管理数据库、Redis 和后台任务的资源生命周期。
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gowms/internal/app"
	"gowms/internal/bootstrap"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/observability"
	"gowms/internal/pkg/snowflake"
)

func main() {
	if err := run(); err != nil {
		slog.Error("wms stopped", "err", err)
		os.Exit(1)
	}
}

// run 管理资源生命周期；返回后 main 才退出，确保已注册的 defer 都能执行。
func run() error {
	if err := loadDotEnv(".env"); err != nil {
		return err
	}
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log.Init(cfg.Log.Level)
	if err := snowflake.Init(cfg.Server.Node); err != nil {
		return fmt.Errorf("init ID generator: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
			log.L().Error("close database failed", "err", err)
		}
	}()
	if cfg.Server.Mode == "debug" {
		if err := bootstrap.Migrate(db, cfg); err != nil {
			return fmt.Errorf("auto migrate: %w", err)
		}
	} else {
		log.L().Info("AutoMigrate disabled in release mode; run cmd/migrate before startup")
		if err := bootstrap.SeedDemoAccounts(db, cfg); err != nil {
			return fmt.Errorf("sync demo accounts: %w", err)
		}
		if err := bootstrap.SeedPersonalAccounts(db, cfg); err != nil {
			return fmt.Errorf("sync personal accounts: %w", err)
		}
	}
	rdb := bootstrap.InitRedis(cfg)
	defer func() {
		if err := rdb.Close(); err != nil {
			log.L().Error("close redis failed", "err", err)
		}
	}()

	var metrics *observability.Metrics
	if cfg.Metrics.Enabled {
		metrics = observability.New(db, "gowms-api")
	}
	application := app.New(cfg, db, rdb, metrics)
	router, err := application.NewRouter()
	if err != nil {
		return fmt.Errorf("build router: %w", err)
	}

	// 后台任务在 HTTP 完成关停后再取消，日志消费者退出后才关闭数据库。
	backgroundCtx, cancelBackground := context.WithCancel(context.Background())
	var workers sync.WaitGroup
	workers.Go(func() { application.SystemService.RunOperLogs(backgroundCtx) })
	workers.Go(func() { application.InboundService.RunCompensator(backgroundCtx) })
	workers.Go(func() { application.InboundService.RunImports(backgroundCtx) })
	defer func() {
		cancelBackground()
		workers.Wait()
	}()

	servers := []*http.Server{{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes:    1 << 20,
	}}
	if metrics != nil {
		mux := http.NewServeMux()
		mux.Handle(cfg.Metrics.Path, metrics.Handler())
		servers = append(servers, &http.Server{
			Addr:              fmt.Sprintf(":%d", cfg.Metrics.Port),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       30 * time.Second,
		})
	}
	return serve(ctx, time.Duration(cfg.Server.ShutdownTimeoutSeconds)*time.Second, servers...)
}
