package bootstrap

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/tenant"
)

// personalPerms 持久体验账号权限集合：与演示账号一致，但【不含 wms:demo】。
// 原因：带 wms:demo 的账号登录后前端会领取演示会话，而那会重置本租户数据；
// 持久账号的数据必须长期保留，因此既不加会话锁、也不参与演示重置。
const personalPerms = "wms:basic,wms:inventory,wms:task," +
	"wms:inbound:view,wms:inbound:create,wms:inbound:submit,wms:inbound:approve,wms:inbound:cancel,wms:inbound:receive,wms:inbound:putaway," +
	"wms:outbound:view,wms:outbound:create,wms:outbound:submit,wms:outbound:approve,wms:outbound:cancel,wms:outbound:pick," +
	"wms:stocktake:view,wms:stocktake:create,wms:stocktake:stocktake,wms:stocktake:approve,wms:stocktake:cancel"

// SeedPersonalAccounts 幂等创建持久体验账号（user1..userN）：每个账号独占一个租户（20001+），
// 数据互不影响；与演示账号的区别是数据长期保留（不重置、无会话锁），
// 且登录页由访客自行挑选账号（见 demo 模块的 /personal/* 接口）。
// 首次创建租户时种入一份初始业务数据，之后任何路径都不会再重置该租户。
// 只有 WMS_PERSONAL_ENABLED=true 时才创建；关闭时禁用历史个人账号。
func SeedPersonalAccounts(db *gorm.DB, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Personal.Enabled || cfg.Personal.Instances <= 0 {
		// 关闭持久账号：禁用历史账号（命名与 demoN 区分，避免误伤演示账号）
		return db.Model(&sysmodel.SysUser{}).
			Where("tenant_id IN ? AND username REGEXP ?", managedPersonalTenantIDs(), "^user[0-9]+$").
			Update("status", 0).Error
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Personal.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	current := make([]string, 0, cfg.Personal.Instances)
	for i := 1; i <= cfg.Personal.Instances; i++ {
		username := cfg.Personal.AccountUsername(i)
		tenantID := cfg.Personal.AccountTenantID(i)
		current = append(current, username)

		// 个人体验角色（每租户一份，幂等：存在则同步权限）
		var role sysmodel.SysRole
		err := db.Where("tenant_id = ? AND name = ?", tenantID, "personal").First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = sysmodel.SysRole{TenantID: tenantID, Name: "personal", Perms: personalPerms, Remark: "公开持久体验账号，数据长期保留"}
			if err := db.Create(&role).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&role).Updates(map[string]any{
			"perms": personalPerms, "remark": "公开持久体验账号，数据长期保留",
		}).Error; err != nil {
			return err
		}

		// 个人体验用户（幂等：存在则同步密码/昵称并启用）
		var user sysmodel.SysUser
		err = db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = sysmodel.SysUser{
				TenantID: tenantID, Username: username, PasswordHash: string(hash),
				Nickname: cfg.Personal.AccountNickname(i), Status: 1,
			}
			if err := db.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&user).Updates(map[string]any{
			"password_hash": string(hash), "nickname": cfg.Personal.AccountNickname(i), "status": 1,
		}).Error; err != nil {
			return err
		}

		// 用户-角色关联（幂等）
		var link sysmodel.SysUserRole
		err = db.Where("user_id = ? AND role_id = ?", user.ID, role.ID).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&sysmodel.SysUserRole{TenantID: tenantID, UserID: user.ID, RoleID: role.ID}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		// 首次创建该租户时种入初始业务数据（seedDemoData 自带幂等：已有仓库即跳过），
		// 之后访客的改动永久保留。
		if err := seedPersonalData(db, tenantID); err != nil {
			return err
		}
	}

	// 禁用不在当前名单内的历史个人账号（收缩数量后多余的 userN+1..）
	return db.Model(&sysmodel.SysUser{}).
		Where("tenant_id IN ? AND username REGEXP ? AND username NOT IN ?",
			managedPersonalTenantIDs(), "^user[0-9]+$", current).
		Update("status", 0).Error
}

func managedPersonalTenantIDs() []int64 {
	ids := make([]int64, 0, 99)
	var cfg config.PersonalConfig
	for i := 1; i <= 99; i++ {
		ids = append(ids, cfg.AccountTenantID(i))
	}
	return ids
}

// seedPersonalData 给持久租户种入初始业务数据；必须在租户 ctx 内执行，
// 否则计数与写入都会落到 tenant_id=0 的平台租户（而非该个人账号的租户）。
func seedPersonalData(db *gorm.DB, tenantID int64) error {
	ctx := tenant.WithTenant(context.Background(), tenantID)
	return db.WithContext(ctx).Transaction(seedDemoData)
}
