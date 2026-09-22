package bootstrap

import (
	"gorm.io/gorm"
)

// Seed 写入内置账号与演示基础数据（幂等：各类数据已存在时分别跳过）。
func Seed(db *gorm.DB) error {
	if err := seedAdmin(db); err != nil {
		return err
	}
	return seedDemoData(db)
}
