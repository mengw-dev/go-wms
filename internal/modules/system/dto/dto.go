package dto

import "gowms/internal/pkg/typex"

type LoginReq struct {
	Username string `json:"username" binding:"required,max=64"`
	Password string `json:"password" binding:"required,max=72"`
	// 省略时仅允许唯一用户名登录；显式 0 只匹配平台账号，不表示跨租户查询。
	TenantID *int64 `json:"tenant_id,string" binding:"omitempty,gte=0"`
}

type LoginResp struct {
	Token    string   `json:"token"`
	UserID   int64    `json:"user_id,string"`
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Roles    []string `json:"roles"`
	Perms    []string `json:"perms"`
}

type ProfileResp struct {
	UserID   int64    `json:"user_id,string"`
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Roles    []string `json:"roles"`
	Perms    []string `json:"perms"`
}

type UserCreateReq struct {
	Username string          `json:"username" binding:"required,max=64"`
	Password string          `json:"password" binding:"required,min=6,max=32"`
	Nickname string          `json:"nickname" binding:"max=64"`
	RoleIDs  typex.Int64List `json:"role_ids"`
}

type UserUpdateReq struct {
	Nickname *string         `json:"nickname" binding:"omitempty,max=64"`
	Status   *int            `json:"status" binding:"omitempty,oneof=0 1"`
	RoleIDs  typex.Int64List `json:"role_ids"`
}

type UserListQuery struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type ResetPwdReq struct {
	Password string `json:"password" binding:"required,min=6,max=32"`
}

type ChangePwdReq struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6,max=32"`
}

type StatusReq struct {
	Status *int `json:"status" binding:"required,oneof=0 1"`
}

type RoleCreateReq struct {
	Name   string `json:"name" binding:"required,max=64"`
	Perms  string `json:"perms" binding:"max=1024"`
	Remark string `json:"remark" binding:"max=255"`
}

type RoleUpdateReq struct {
	Name   string `json:"name" binding:"required,max=64"`
	Perms  string `json:"perms" binding:"max=1024"`
	Remark string `json:"remark" binding:"max=255"`
}

type RoleListQuery struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type OperLogQuery struct {
	Username string `form:"username"`
	Path     string `form:"path"`
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
}
