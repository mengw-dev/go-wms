package model

import (
	"time"

	"gorm.io/gorm"
)

// Base 所有业务表通用字段。
type Base struct {
	ID        int64          `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// Versioned 在 Base 上增加乐观锁，用于单据防并发状态跳变。
type Versioned struct {
	Version int `json:"version" gorm:"default:1"`
}

type SysUser struct {
	Base
	Username     string `json:"username" gorm:"size:64;uniqueIndex;not null"`
	PasswordHash string `json:"-" gorm:"size:128;not null"`
	Nickname     string `json:"nickname" gorm:"size:64"`
	Status       int    `json:"status" gorm:"default:1"`     // 1 启用 0 禁用
	TokenVersion int    `json:"-" gorm:"not null;default:1"` // 修改密码/禁用后使旧 Token 失效
}

func (SysUser) TableName() string { return "sys_user" }

type SysRole struct {
	Base
	Name   string `json:"name" gorm:"size:64;uniqueIndex;not null"`
	Perms  string `json:"perms" gorm:"size:1024"` // 逗号分隔，如 wms:inbound:approve；* 表示全部
	Remark string `json:"remark" gorm:"size:255"`
}

func (SysRole) TableName() string { return "sys_role" }

type SysUserRole struct {
	ID     int64 `gorm:"primaryKey"`
	UserID int64 `gorm:"uniqueIndex:uk_user_role"`
	RoleID int64 `gorm:"uniqueIndex:uk_user_role"`
}

func (SysUserRole) TableName() string { return "sys_user_role" }

type SysOperLog struct {
	ID        int64     `json:"id" gorm:"primaryKey"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username" gorm:"size:64"`
	Path      string    `json:"path" gorm:"size:255"`
	Method    string    `json:"method" gorm:"size:16"`
	Params    string    `json:"params" gorm:"type:text"`
	IP        string    `json:"ip" gorm:"size:64"`
	CostMs    int64     `json:"cost_ms"`
	Status    int       `json:"status"`
	Result    string    `json:"result" gorm:"type:text"`
	CreatedAt time.Time `json:"created_at" gorm:"index"`
}

func (SysOperLog) TableName() string { return "sys_oper_log" }
