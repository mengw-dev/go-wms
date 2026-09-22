// Package bootstrap 负责数据库与 Redis 初始化、开发环境迁移，以及演示数据的种子与重置。
package bootstrap

import (
	"gorm.io/gorm"

	"gowms/internal/pkg/config"
)

// Migrate 仅供开发和测试使用：AutoMigrate + 种子数据。
func Migrate(db *gorm.DB, cfg *config.Config) error {
	if err := AutoMigrate(db); err != nil {
		return err
	}
	if err := Seed(db); err != nil {
		return err
	}
	if err := SeedDemoAccounts(db, cfg); err != nil {
		return err
	}
	return SeedPersonalAccounts(db, cfg)
}
