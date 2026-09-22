package bootstrap

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
)

// demoPerms 演示账号权限集合（只读 + 业务操作 + 演示中心，无系统管理权限）。
const demoPerms = "wms:basic,wms:inventory,wms:task," +
	"wms:inbound:view,wms:inbound:create,wms:inbound:submit,wms:inbound:approve,wms:inbound:cancel,wms:inbound:receive,wms:inbound:putaway," +
	"wms:outbound:view,wms:outbound:create,wms:outbound:submit,wms:outbound:approve,wms:outbound:cancel,wms:outbound:pick," +
	"wms:stocktake:view,wms:stocktake:create,wms:stocktake:stocktake,wms:stocktake:approve,wms:stocktake:cancel,wms:demo"

// SeedDemoAccounts 幂等创建多演示账号（demo1..demoN）：每个账号独占一个租户（10001+）
// 与一份该租户内的演示角色（角色查询走租户过滤，必须与用户同租户），数据互不影响。
// 同时禁用不在当前名单内的历史演示账号（数量收缩后多余的账号、旧的单 demo 账号），
// 避免旧账号绕过按租户的会话锁。只有 WMS_DEMO_ENABLED=true 时才创建。
func SeedDemoAccounts(db *gorm.DB, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Demo.Enabled || cfg.Demo.Instances <= 0 {
		// 关闭演示模式：禁用全部历史演示账号，避免旧账号绕过 DemoSession 中间件。
		return db.Model(&sysmodel.SysUser{}).
			Where("tenant_id IN ? AND (username = ? OR username REGEXP ?)", managedDemoTenantIDs(), "demo", "^demo[0-9]+$").
			Update("status", 0).Error
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Demo.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	current := make([]string, 0, cfg.Demo.Instances)
	for i := 1; i <= cfg.Demo.Instances; i++ {
		username := cfg.Demo.AccountUsername(i)
		tenantID := cfg.Demo.AccountTenantID(i)
		nickname := fmt.Sprintf("演示访客%d", i)
		current = append(current, username)

		// 演示角色（每租户一份，幂等：存在则同步权限）
		var role sysmodel.SysRole
		err := db.Where("tenant_id = ? AND name = ?", tenantID, "demo").First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = sysmodel.SysRole{TenantID: tenantID, Name: "demo", Perms: demoPerms, Remark: "公开业务体验账号，仅允许业务操作"}
			if err := db.Create(&role).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&role).Updates(map[string]any{
			"perms": demoPerms, "remark": "公开业务体验账号，仅允许业务操作",
		}).Error; err != nil {
			return err
		}

		// 演示用户（幂等：存在则同步密码/昵称并启用）
		var user sysmodel.SysUser
		err = db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = sysmodel.SysUser{
				TenantID: tenantID, Username: username, PasswordHash: string(hash),
				Nickname: nickname, Status: 1,
			}
			if err := db.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&user).Updates(map[string]any{
			"password_hash": string(hash), "nickname": nickname, "status": 1,
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
	}

	// 禁用不在当前名单内的历史演示账号（收缩数量后多余的 demoN+1.. 与旧 demo）
	return db.Model(&sysmodel.SysUser{}).
		Where("tenant_id IN ? AND (username = ? OR username REGEXP ?) AND username NOT IN ?",
			managedDemoTenantIDs(), "demo", "^demo[0-9]+$", current).
		Update("status", 0).Error
}

func managedDemoTenantIDs() []int64 {
	ids := make([]int64, 0, 99)
	var cfg config.DemoConfig
	for i := 1; i <= 99; i++ {
		ids = append(ids, cfg.AccountTenantID(i))
	}
	return ids
}
