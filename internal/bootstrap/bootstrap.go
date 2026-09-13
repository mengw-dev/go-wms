package bootstrap

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
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
)

// InitDB 初始化 GORM MySQL 连接。
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
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// InitRedis 初始化 Redis（不可用时仅记录告警，业务自动降级）。
func InitRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unavailable, order generator will fallback to local mode", "err", err)
	}
	return rdb
}

// Migrate 自动迁移表结构 + CHECK 约束 + 种子数据。
func Migrate(db *gorm.DB) error {
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
	return seed(db)
}

// seed 内置管理员与角色：admin / admin123。
func seed(db *gorm.DB) error {
	var n int64

	if err := db.Model(&sysmodel.SysUser{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	admin := &sysmodel.SysUser{Username: "admin", PasswordHash: string(hash), Nickname: "管理员", Status: 1}
	role := &sysmodel.SysRole{Name: "admin", Perms: "*", Remark: "内置超级管理员"}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(admin).Error; err != nil {
			return err
		}
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		return tx.Create(&sysmodel.SysUserRole{UserID: admin.ID, RoleID: role.ID}).Error
	})
}
