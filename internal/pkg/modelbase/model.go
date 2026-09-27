// Package modelbase 定义跨业务模块共享的持久化基础字段。
package modelbase

import (
	"time"

	"gorm.io/gorm"
)

// Base 是业务表通用字段。
type Base struct {
	ID        int64          `json:"id,string" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// Versioned 增加内部修订号；是否执行乐观锁校验取决于具体 SQL 是否比较旧版本。
type Versioned struct {
	Version int `json:"-" gorm:"default:1"`
}
