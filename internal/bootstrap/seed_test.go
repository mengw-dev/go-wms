package bootstrap

import (
	"os"
	"testing"

	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/testutil"
)

func seedAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	return testutil.OpenIsolatedMySQL(t, dsn, &sysmodel.SysUser{}, &sysmodel.SysRole{}, &sysmodel.SysUserRole{})
}

func TestSeedAdminCreatesAndRepairsPartialState(t *testing.T) {
	db := seedAdminTestDB(t)
	// 先有一个普通租户用户，旧逻辑会因为“已有用户”而跳过管理员初始化。
	if err := db.Create(&sysmodel.SysUser{
		Base: sysmodel.Base{ID: 1}, TenantID: 42, Username: "regular",
		PasswordHash: "unused", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := seedAdmin(db); err != nil {
		t.Fatal(err)
	}
	if err := seedAdmin(db); err != nil {
		t.Fatal(err)
	}

	var admin sysmodel.SysUser
	if err := db.Where("tenant_id = ? AND username = ?", 0, "admin").First(&admin).Error; err != nil {
		t.Fatal(err)
	}
	var role sysmodel.SysRole
	if err := db.Where("tenant_id = ? AND name = ?", 0, "admin").First(&role).Error; err != nil {
		t.Fatal(err)
	}
	var links int64
	if err := db.Model(&sysmodel.SysUserRole{}).
		Where("tenant_id = ? AND user_id = ? AND role_id = ?", 0, admin.ID, role.ID).
		Count(&links).Error; err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("admin links=%d want=1", links)
	}

	// 模拟管理员角色或关联被误删，初始化必须能补回，而不是因为用户已存在直接跳过。
	if err := db.Where("tenant_id = ? AND user_id = ? AND role_id = ?", 0, admin.ID, role.ID).
		Delete(&sysmodel.SysUserRole{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Unscoped().Where("tenant_id = ? AND name = ?", 0, "admin").Delete(&sysmodel.SysRole{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := seedAdmin(db); err != nil {
		t.Fatal(err)
	}
	var roles []sysmodel.SysRole
	if err := db.Unscoped().Where("tenant_id = ? AND name = ?", 0, "admin").Find(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 {
		t.Fatalf("admin roles after repair=%+v", roles)
	}
	role = roles[0]
	if err := db.Model(&sysmodel.SysUserRole{}).
		Where("tenant_id = ? AND user_id = ? AND role_id = ?", 0, admin.ID, role.ID).
		Count(&links).Error; err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("repaired admin links=%d want=1", links)
	}
}

func TestSeedDemoAccountsDoesNotDisableSameNameInOtherTenant(t *testing.T) {
	db := seedAdminTestDB(t)
	if err := db.Create(&sysmodel.SysUser{
		TenantID: 42, Username: "demo9", PasswordHash: "unused", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Demo: config.DemoConfig{Enabled: false, Instances: 5}}
	if err := SeedDemoAccounts(db, cfg); err != nil {
		t.Fatal(err)
	}
	var user sysmodel.SysUser
	if err := db.Where("tenant_id = ? AND username = ?", 42, "demo9").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Status != 1 {
		t.Fatal("closing demo accounts disabled an ordinary tenant account with the same name")
	}
}
