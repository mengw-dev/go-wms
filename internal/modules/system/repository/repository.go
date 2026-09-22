// Package repository 提供 system 模块的用户、角色、权限和操作日志持久化操作。
package repository

import (
	"errors"

	"gorm.io/gorm"
)

var (
	ErrInvalidRole   = errors.New("role is missing or belongs to another tenant")
	ErrRoleInUse     = errors.New("role is assigned to a user")
	ErrAmbiguousUser = errors.New("username exists in more than one tenant")
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }
