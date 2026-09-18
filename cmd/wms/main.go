package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gowms/internal/app"
	"gowms/internal/bootstrap"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/observability"
)

// loadDotEnv 本地原生开发便利：若项目根存在 .env（已 gitignore）则注入进程环境变量，
// 使 ZHIPU_API_KEY 等密钥与 Docker 部署（compose --env-file 注入）行为一致。
// 真实环境变量优先于 .env；容器内没有 .env 文件时自动跳过，不影响生产。
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // 无 .env 属正常（CI/容器/纯环境变量部署）
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

func main() {
	// 0. 本地开发：加载 .env（存在时），密钥不进代码库
	loadDotEnv(".env")
	// 1. 加载配置
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		slog.Error("load config failed", "err", err)
		os.Exit(1)
	}
	// 2. 日志
	log.Init(cfg.Log.Level)
	logger := log.L()

	// 3. DB / Redis / 迁移
	db, err := bootstrap.InitDB(cfg)
	if err != nil {
		logger.Error("init db failed", "err", err)
		os.Exit(1)
	}
	if cfg.Server.Mode == "debug" {
		if err := bootstrap.Migrate(db, cfg); err != nil {
			logger.Error("auto migrate failed", "err", err)
			os.Exit(1)
		}
	} else {
		logger.Info("AutoMigrate disabled in release mode; run cmd/migrate before startup")
		// 每次启动同步演示账号状态：开启时创建/修复，关闭时禁用旧账号。
		if err := bootstrap.SeedDemoAccount(db, cfg); err != nil {
			logger.Error("sync demo account failed", "err", err)
			os.Exit(1)
		}
	}
	rdb := bootstrap.InitRedis(cfg)

	sqlDB, err := db.DB()
	if err != nil {
		logger.Error("get sql db failed", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()
	defer rdb.Close()

	// 4. 组装依赖
	var metrics *observability.Metrics
	if cfg.Metrics.Enabled {
		metrics = observability.New(db, "gowms-api")
	}
	application, err := app.New(cfg, db, rdb, metrics)
	if err != nil {
		logger.Error("assemble app failed", "err", err)
		os.Exit(1)
	}
	// 5. 后台任务：Excel 导入悬挂补偿（每 2 分钟扫描；随服务关停退出）
	compensatorCtx, compensatorCancel := context.WithCancel(context.Background())
	defer compensatorCancel()
	application.InboundService.StartCompensator(compensatorCtx)

	// 6. 启动 HTTP 服务（优雅关停）
	router, err := application.NewRouter()
	if err != nil {
		logger.Error("build router failed", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("wms server started", "port", cfg.Server.Port, "mode", cfg.Server.Mode)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	var metricsSrv *http.Server
	if metrics != nil {
		mux := http.NewServeMux()
		mux.Handle(cfg.Metrics.Path, metrics.Handler())
		metricsSrv = &http.Server{
			Addr:              fmt.Sprintf(":%d", cfg.Metrics.Port),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       30 * time.Second,
		}
		go func() {
			logger.Info("metrics server started", "port", cfg.Metrics.Port, "path", cfg.Metrics.Path)
			if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				serverErr <- fmt.Errorf("metrics server: %w", err)
			}
		}()
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		logger.Info("shutting down...")
	case err := <-serverErr:
		logger.Error("server exited", "err", err)
		compensatorCancel()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown failed", "err", err)
	}
	if metricsSrv != nil {
		if err := metricsSrv.Shutdown(ctx); err != nil {
			logger.Error("metrics shutdown failed", "err", err)
		}
	}
	compensatorCancel() // HTTP 关停后停止补偿扫描
	logger.Info("bye")
}
