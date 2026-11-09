package repository

import (
	"context"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/system/model"
)

// Transaction supplies a repository bound to the caller's short administration
// transaction. Use the InTx mutations below to avoid nested transactions.
func (r *Repository) Transaction(ctx context.Context, fn func(*Repository) error) error {
	return r.db.WithContext(ctx).Transaction(func(db *gorm.DB) error { return fn(New(db)) })
}

// LockUsers serializes authority/target changes before any role locks are taken.
// All administration commands acquire users by ID, then roles by ID.
func (r *Repository) LockUsers(ctx context.Context, ids []int64) ([]*model.SysUser, error) {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var users []*model.SysUser
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id").Find(&users).Error
	return users, err
}

// UserRoleIDs runs after LockUsers. Role association mutations lock their user
// first, so these links cannot change until this transaction releases the users.
func (r *Repository) UserRoleIDs(ctx context.Context, users []*model.SysUser) error {
	return r.attachUserRoleIDs(ctx, users)
}

func (r *Repository) LockRoles(ctx context.Context, ids []int64) ([]*model.SysRole, error) {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	var roles []*model.SysRole
	if len(ids) == 0 {
		return roles, nil
	}
	err := r.db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id").Find(&roles).Error
	return roles, err
}
