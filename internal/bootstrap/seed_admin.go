package bootstrap

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
)

// devAdminPassword 开发模式（debug）内置管理员的默认密码。
// release 模式首次创建管理员必须显式配置 WMS_ADMIN_PASSWORD，不会回退到该值。
const devAdminPassword = "admin123"

// SeedAdmin 写入/修复内置管理员、角色与用户角色关联（幂等）。
// 首次创建管理员时密码取 WMS_ADMIN_PASSWORD：release 模式未配置则直接失败，
// 避免公开部署出现默认口令；管理员已存在时不重新读取密码（重启不需要再次提供）。
func SeedAdmin(db *gorm.DB, cfg *config.Config) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var admin sysmodel.SysUser
		err := tx.Where("tenant_id = ? AND username = ?", 0, "admin").First(&admin).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			password, pwdErr := adminPassword(cfg)
			if pwdErr != nil {
				return pwdErr
			}
			hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
			if hashErr != nil {
				return hashErr
			}
			admin = sysmodel.SysUser{TenantID: 0, Username: "admin", PasswordHash: string(hash), Nickname: "管理员", Status: 1}
			if err := tx.Create(&admin).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var role sysmodel.SysRole
		err = tx.Where("tenant_id = ? AND name = ?", 0, "admin").First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = sysmodel.SysRole{TenantID: 0, Name: "admin", Perms: "*", Remark: "内置超级管理员"}
			if err := tx.Create(&role).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var link sysmodel.SysUserRole
		err = tx.Where("tenant_id = ? AND user_id = ? AND role_id = ?", 0, admin.ID, role.ID).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&sysmodel.SysUserRole{TenantID: 0, UserID: admin.ID, RoleID: role.ID}).Error
		}
		return err
	})
}

// adminPassword 解析首次创建管理员使用的密码：显式配置优先；
// 未配置时 release 拒绝创建，debug 回退到开发默认值。
func adminPassword(cfg *config.Config) (string, error) {
	password := ""
	release := false
	if cfg != nil {
		password = strings.TrimSpace(cfg.Admin.Password)
		release = cfg.Server.Mode == "release"
	}
	if password != "" {
		return password, nil
	}
	if release {
		return "", errors.New("release mode requires WMS_ADMIN_PASSWORD to create the built-in admin account")
	}
	return devAdminPassword, nil
}
