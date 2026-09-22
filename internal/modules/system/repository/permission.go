package repository

import (
	"context"

	"gowms/internal/modules/system/model"
)

func (r *Repository) ListRoleNamesByUser(ctx context.Context, userID int64) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).Model(&model.SysRole{}).
		Joins("JOIN sys_user_role ur ON ur.role_id = sys_role.id AND ur.tenant_id = sys_role.tenant_id").
		Joins("JOIN sys_user u ON u.id = ur.user_id AND u.tenant_id = sys_role.tenant_id AND u.deleted_at IS NULL").
		Where("ur.user_id = ?", userID).
		Pluck("sys_role.name", &names).Error
	return names, err
}

// GetPermsByUser 汇总用户所有角色的权限标识。
func (r *Repository) GetPermsByUser(ctx context.Context, userID int64) ([]string, error) {
	var perms []string
	err := r.db.WithContext(ctx).Model(&model.SysRole{}).
		Joins("JOIN sys_user_role ur ON ur.role_id = sys_role.id AND ur.tenant_id = sys_role.tenant_id").
		Joins("JOIN sys_user u ON u.id = ur.user_id AND u.tenant_id = sys_role.tenant_id AND u.deleted_at IS NULL").
		Where("ur.user_id = ? AND sys_role.deleted_at IS NULL", userID).
		Pluck("sys_role.perms", &perms).Error
	return perms, err
}
