package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"gowms/internal/modules/basic/model"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/tenant"
)

// InitDB 初始化 GORM MySQL 连接，并注册多租户全局回调。
func InitDB(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.Server.Mode == "debug" {
		logLevel = logger.Info
	}
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 表名单数，与 TableName() 一致
		},
	})
	if err != nil {
		return nil, err
	}
	// 多租户全局隔离：ctx 有租户时自动注入 WHERE tenant_id = ?；
	// 迁移/种子等 context.Background() 场景天然旁路（tenant_id=0）。
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		return nil, fmt.Errorf("register tenant callbacks: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// InitRedis 初始化 Redis。缓存和单号可以降级；会话校验等安全操作仍需拒绝故障请求。
func InitRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
		ContextTimeoutEnabled: true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unavailable, order generator will fallback to local mode", "err", err)
	}
	return rdb
}

// AutoMigrate 自动迁移表结构 + CHECK 约束。生产环境应使用 cmd/migrate。
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&sysmodel.SysUser{}, &sysmodel.SysRole{}, &sysmodel.SysUserRole{}, &sysmodel.SysOperLog{},
		&model.Warehouse{}, &model.Location{}, &model.SKU{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{},
		&taskmodel.Task{},
		&inboundmodel.ReceiptOrder{}, &inboundmodel.ReceiptOrderDetail{}, &inboundmodel.ImportTask{},
		&outboundmodel.ShipmentOrder{}, &outboundmodel.ShipmentOrderDetail{}, &outboundmodel.Allocation{},
		&stocktakemodel.StocktakeOrder{}, &stocktakemodel.StocktakeDetail{},
	); err != nil {
		return err
	}
	// CHECK 约束（MySQL 8.0.16+ 强制执行）；已存在时报 1061 duplicate，属预期可忽略
	if err := db.Exec("ALTER TABLE wms_inventory ADD CONSTRAINT chk_inv_non_negative CHECK (available_quantity >= 0 AND stock_quantity >= 0)").Error; err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
			!strings.Contains(err.Error(), "1061") {
			log.L().Warn("add inventory check constraint failed", "err", err)
		}
	}
	return nil
}
