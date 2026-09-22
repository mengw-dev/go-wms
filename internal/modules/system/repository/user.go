package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/typex"
)

func (r *Repository) GetUserByUsername(ctx context.Context, username string) (*model.SysUser, error) {
	var u model.SysUser
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND username = ?", tenant.FromContext(ctx), username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetLoginUser 兼容不传租户的旧登录请求，但不从同名账号中任意挑选一个。
func (r *Repository) GetLoginUser(ctx context.Context, username string, tenantID *int64) (*model.SysUser, error) {
	q := r.db.WithContext(ctx).Where("username = ?", username)
	if tenantID != nil {
		// tenant_id=0 的账号同样需要精确匹配，不能使用平台旁路语义。
		q = q.Where("tenant_id = ?", *tenantID)
	}
	var users []model.SysUser
	if err := q.Limit(2).Find(&users).Error; err != nil {
		return nil, err
	}
	switch len(users) {
	case 0:
		return nil, gorm.ErrRecordNotFound
	case 1:
		return &users[0], nil
	default:
		return nil, ErrAmbiguousUser
	}
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
		return replaceRoles(tx, u.ID, u.TenantID, roleIDs)
	})
}

func (r *Repository) UpdateUser(ctx context.Context, id int64, nickname *string, status *int, roleIDs []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var user model.SysUser
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error; err != nil {
			return err
		}
		updates := make(map[string]any)
		if nickname != nil {
			updates["nickname"] = *nickname
		}
		if status != nil {
			updates["status"] = *status
			// 禁用再启用后旧 Token 也不能恢复有效。
			updates["token_version"] = gorm.Expr("token_version + 1")
		}
		if len(updates) > 0 {
			if err := tx.Model(&model.SysUser{}).Where("id = ?", id).Updates(updates).Error; err != nil {
				return err
			}
		}
		if roleIDs != nil {
			return replaceRoles(tx, id, user.TenantID, roleIDs)
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
		q = q.Where("username LIKE ? OR nickname LIKE ?", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SysUser
	if err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error; err != nil {
		return nil, 0, err
	}
	if err := r.attachUserRoleIDs(ctx, list); err != nil {
		return nil, 0, err
	}
	return list, total, nil
}

func (r *Repository) attachUserRoleIDs(ctx context.Context, users []*model.SysUser) error {
	if len(users) == 0 {
		return nil
	}
	userIDs := make([]int64, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.ID)
	}

	var rows []struct {
		UserID int64 `gorm:"column:user_id"`
		RoleID int64 `gorm:"column:role_id"`
	}
	if err := r.db.WithContext(ctx).Model(&model.SysUserRole{}).
		Joins("JOIN sys_user u ON u.id = sys_user_role.user_id AND u.tenant_id = sys_user_role.tenant_id AND u.deleted_at IS NULL").
		Joins("JOIN sys_role r ON r.id = sys_user_role.role_id AND r.tenant_id = u.tenant_id AND r.deleted_at IS NULL").
		Select("user_id, role_id").
		Where("user_id IN ?", userIDs).
		Order("role_id").
		Scan(&rows).Error; err != nil {
		return err
	}

	roleIDsByUser := make(map[int64]typex.Int64List, len(users))
	for _, row := range rows {
		roleIDsByUser[row.UserID] = append(roleIDsByUser[row.UserID], row.RoleID)
	}
	for _, user := range users {
		user.RoleIDs = roleIDsByUser[user.ID]
		if user.RoleIDs == nil {
			user.RoleIDs = typex.Int64List{}
		}
	}
	return nil
}
