package bootstrap

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
)

// seedAdmin 内置管理员与角色：admin / admin123。
func seedAdmin(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var admin sysmodel.SysUser
		err := tx.Where("tenant_id = ? AND username = ?", 0, "admin").First(&admin).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
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
