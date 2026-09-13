package repository

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/system/model"
)

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

// ---------- 用户 ----------

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*model.SysUser, error) {
	var u model.SysUser
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (*model.SysUser, error) {
	var u model.SysUser
	if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) CreateUser(ctx context.Context, u *model.SysUser, roleIDs []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(u).Error; err != nil {
			return err
		}
		return replaceRoles(tx, u.ID, roleIDs)
	})
}

func (r *Repository) UpdateUser(ctx context.Context, id int64, nickname string, status *int, roleIDs []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"nickname": nickname}
		if status != nil {
			updates["status"] = *status
			// 禁用再启用后旧 Token 也不能恢复有效。
			updates["token_version"] = gorm.Expr("token_version + 1")
		}
		if err := tx.Model(&model.SysUser{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if roleIDs != nil {
			return replaceRoles(tx, id, roleIDs)
		}
		return nil
	})
}

func (r *Repository) DeleteUser(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&model.SysUser{}, id).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", id).Delete(&model.SysUserRole{}).Error
	})
}

func (r *Repository) UpdatePassword(ctx context.Context, id int64, hash string) error {
	return r.db.WithContext(ctx).Model(&model.SysUser{}).Where("id = ?", id).
		Updates(map[string]any{
			"password_hash": hash,
			"token_version": gorm.Expr("token_version + 1"),
		}).Error
}

func (r *Repository) ListUsers(ctx context.Context, keyword string, page, size int) ([]*model.SysUser, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.SysUser{})
	if keyword != "" {
		q = q.Where("username LIKE ? OR nickname LIKE ?", "%"+keyword+"%", "%"+keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SysUser
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// ---------- 角色 ----------

func (r *Repository) GetRoleByName(ctx context.Context, name string) (*model.SysRole, error) {
	var role model.SysRole
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) CreateRole(ctx context.Context, role *model.SysRole) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *Repository) UpdateRole(ctx context.Context, id int64, name, perms, remark string) error {
	return r.db.WithContext(ctx).Model(&model.SysRole{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "perms": perms, "remark": remark}).Error
}

func (r *Repository) DeleteRole(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.SysRole{}, id).Error
}

func (r *Repository) CountUserRole(ctx context.Context, roleID int64) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&model.SysUserRole{}).Where("role_id = ?", roleID).Count(&n).Error
	return n, err
}

func (r *Repository) ListRoles(ctx context.Context, keyword string, page, size int) ([]*model.SysRole, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.SysRole{})
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+keyword+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SysRole
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

func (r *Repository) ListAllRoles(ctx context.Context) ([]*model.SysRole, error) {
	var list []*model.SysRole
	err := r.db.WithContext(ctx).Order("id").Find(&list).Error
	return list, err
}

// ---------- 用户-角色 ----------

func replaceRoles(tx *gorm.DB, userID int64, roleIDs []int64) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.SysUserRole{}).Error; err != nil {
		return err
	}
	for _, rid := range roleIDs {
		if err := tx.Create(&model.SysUserRole{UserID: userID, RoleID: rid}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListRoleNamesByUser(ctx context.Context, userID int64) ([]string, error) {
	var names []string
	err := r.db.WithContext(ctx).Model(&model.SysRole{}).
		Joins("JOIN sys_user_role ur ON ur.role_id = sys_role.id").
		Where("ur.user_id = ?", userID).
		Pluck("sys_role.name", &names).Error
	return names, err
}

// GetPermsByUser 汇总用户所有角色的权限标识。
func (r *Repository) GetPermsByUser(ctx context.Context, userID int64) ([]string, error) {
	var perms []string
	err := r.db.WithContext(ctx).Model(&model.SysRole{}).
		Joins("JOIN sys_user_role ur ON ur.role_id = sys_role.id").
		Where("ur.user_id = ? AND sys_role.deleted_at IS NULL", userID).
		Pluck("sys_role.perms", &perms).Error
	return perms, err
}

// ---------- 操作日志 ----------

func (r *Repository) InsertOperLogs(ctx context.Context, logs []*model.SysOperLog) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(logs, 100).Error
}

func (r *Repository) ListOperLogs(ctx context.Context, username, path string, page, size int) ([]*model.SysOperLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.SysOperLog{})
	if username != "" {
		q = q.Where("username = ?", username)
	}
	if path != "" {
		q = q.Where("path LIKE ?", "%"+path+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SysOperLog
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}
