package repository

import (
	"context"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/dbutil"
)

func (r *Repository) GetRoleByName(ctx context.Context, name string) (*model.SysRole, error) {
	var role model.SysRole
	if err := r.db.WithContext(ctx).Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *Repository) GetRoleByID(ctx context.Context, id int64) (*model.SysRole, error) {
	var role model.SysRole
	if err := r.db.WithContext(ctx).First(&role, id).Error; err != nil {
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
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 分配角色也先锁角色行，保证“未被使用”的判断与删除之间不能插入新关联。
		var role model.SysRole
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&role, id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.SysUserRole{}).Where("role_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRoleInUse
		}
		return tx.Delete(&role).Error
	})
}

func (r *Repository) ListRoles(ctx context.Context, keyword string, page, size int) ([]*model.SysRole, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.SysRole{})
	if keyword != "" {
		q = q.Where("name LIKE ?", "%"+dbutil.LikePattern(keyword)+"%")
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

func replaceRoles(tx *gorm.DB, userID, tenantID int64, roleIDs []int64) error {
	ids := slices.Clone(roleIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > 0 {
		if ids[0] <= 0 {
			return ErrInvalidRole
		}
		var roles []model.SysRole
		// 平台代操作也按用户所属租户核对，不能只依赖请求 context 的回调。
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ? AND tenant_id = ?", ids, tenantID).Order("id").Find(&roles).Error; err != nil {
			return err
		}
		if len(roles) != len(ids) {
			return ErrInvalidRole
		}
	}
	if err := tx.Where("user_id = ?", userID).Delete(&model.SysUserRole{}).Error; err != nil {
		return err
	}
	links := make([]model.SysUserRole, 0, len(ids))
	for _, rid := range ids {
		links = append(links, model.SysUserRole{TenantID: tenantID, UserID: userID, RoleID: rid})
	}
	if len(links) == 0 {
		return nil
	}
	return tx.CreateInBatches(links, 100).Error
}
