package bootstrap

import (
	"gorm.io/gorm"

	"gowms/internal/pkg/config"
)

// SeedDemo 写入开发/演示数据：演示基础资料（仓库、库位、SKU、库存、单据）
// 与公开体验账号（demo1..demoN、user1..userN），全部幂等。
// 仅用于开发/演示环境；release 模式的初始化由 cmd/migrate 拒绝执行。
func SeedDemo(db *gorm.DB, cfg *config.Config) error {
	if err := seedDemoData(db); err != nil {
		return err
	}
	if err := SeedDemoAccounts(db, cfg); err != nil {
		return err
	}
	return SeedPersonalAccounts(db, cfg)
}
