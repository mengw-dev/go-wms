package model

import (
	"time"

	"gowms/internal/pkg/typex"

	"gorm.io/gorm"
)

// Base 所有业务表通用字段。
type Base struct {
	ID        int64          `json:"id,string" gorm:"primaryKey"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// Versioned 在 Base 上增加乐观锁，用于单据防并发状态跳变。
type Versioned struct {
	Version int `json:"version" gorm:"default:1"`
}

// SysUser 用户（多租户：tenant_id 联合唯一用户名，各租户内独立）。
type SysUser struct {
	Base
	TenantID     int64           `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_user_username,priority:1"`
	Username     string          `json:"username" gorm:"size:64;uniqueIndex:uk_user_username,priority:2;not null"`
	PasswordHash string          `json:"-" gorm:"size:128;not null"`
	Nickname     string          `json:"nickname" gorm:"size:64"`
	Status       int             `json:"status" gorm:"default:1"`     // 1 启用 0 禁用
	TokenVersion int             `json:"-" gorm:"not null;default:1"` // 修改密码/禁用后使旧 Token 失效
	RoleIDs      typex.Int64List `json:"role_ids" gorm:"-"`
}

func (SysUser) TableName() string { return "sys_user" }

// SysRole 角色（tenant_id 联合唯一角色名）。
type SysRole struct {
	Base
	TenantID int64  `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_role_name,priority:1"`
	Name     string `json:"name" gorm:"size:64;uniqueIndex:uk_role_name,priority:2;not null"`
	Perms    string `json:"perms" gorm:"size:1024"` // 逗号分隔，如 wms:inbound:approve；* 表示全部
	Remark   string `json:"remark" gorm:"size:255"`
}

func (SysRole) TableName() string { return "sys_role" }

// SysUserRole 用户-角色关联（user_id 全局唯一，天然按租户隔离，无需 tenant 复合）。
type SysUserRole struct {
	ID       int64 `gorm:"primaryKey"`
	TenantID int64 `json:"tenant_id,string" gorm:"not null;default:0"`
	UserID   int64 `gorm:"uniqueIndex:uk_user_role"`
	RoleID   int64 `gorm:"uniqueIndex:uk_user_role"`
}

func (SysUserRole) TableName() string { return "sys_user_role" }

type SysOperLog struct {
	ID        int64     `json:"id,string" gorm:"primaryKey"`
	TenantID  int64     `json:"tenant_id,string" gorm:"not null;default:0;index:idx_oper_log_tenant"`
	UserID    int64     `json:"user_id,string"`
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
