package dto

import "gowms/internal/pkg/typex"

type LoginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
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
	Nickname string          `json:"nickname" binding:"max=64"`
	Status   *int            `json:"status"`
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
	Status *int `json:"status" binding:"required"`
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
