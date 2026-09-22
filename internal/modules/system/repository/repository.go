package repository

import (
	"context"
	"errors"
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/typex"
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

// ---------- 用户 ----------

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

// ---------- 角色 ----------

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
		q = q.Where("path LIKE ?", "%"+dbutil.LikePattern(path)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SysOperLog
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}
