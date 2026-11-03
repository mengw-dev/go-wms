package bootstrap

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
)

// syncManagedAccount keeps configuration authoritative for public accounts.
// Compare passwords outside row locks and keep the existing hash on a no-op
// restart; bcrypt's random salt is not a credential change. A conditional write
// prevents a concurrent reset/disable from being silently overwritten using an
// old snapshot. Retry only after reading and comparing the new state.
func syncManagedAccount(db *gorm.DB, user *sysmodel.SysUser, password, hash, nickname string) error {
	for range 3 {
		passwordChanged := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil
		securityChanged := passwordChanged || user.Status != 1
		if !securityChanged && user.Nickname == nickname {
			return nil
		}
		updates := map[string]any{"nickname": nickname, "status": 1}
		if passwordChanged {
			updates["password_hash"] = hash
		}
		if securityChanged {
			updates["token_version"] = gorm.Expr("token_version + 1")
		}
		result := db.Model(&sysmodel.SysUser{}).
			Where("id = ? AND tenant_id = ? AND username = ? AND status = ? AND token_version = ? AND BINARY password_hash = ?",
				user.ID, user.TenantID, user.Username, user.Status, user.TokenVersion, user.PasswordHash).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			return nil
		}
		var current sysmodel.SysUser
		if err := db.Where("id = ? AND tenant_id = ? AND username = ?", user.ID, user.TenantID, user.Username).First(&current).Error; err != nil {
			return err
		}
		*user = current
	}
	return errcode.Conflict
}
