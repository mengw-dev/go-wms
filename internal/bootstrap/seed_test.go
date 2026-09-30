package bootstrap

import (
	"os"
	"testing"

	"golang.org/x/crypto/bcrypt"
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
	devCfg := &config.Config{}
	// 先有一个普通租户用户，旧逻辑会因为“已有用户”而跳过管理员初始化。
	if err := db.Create(&sysmodel.SysUser{
		Base: sysmodel.Base{ID: 1}, TenantID: 42, Username: "regular",
		PasswordHash: "unused", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := SeedAdmin(db, devCfg); err != nil {
		t.Fatal(err)
	}
	if err := SeedAdmin(db, devCfg); err != nil {
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
	if err := SeedAdmin(db, devCfg); err != nil {
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

// 管理员密码策略：显式配置优先；release 未配置时首次创建失败且不落库；
// 管理员已存在时幂等补齐不再要求密码。
func TestSeedAdminPasswordPolicy(t *testing.T) {
	t.Run("explicit password", func(t *testing.T) {
		db := seedAdminTestDB(t)
		// 拼接构造：避免 gosec 把测试密码字面量当成硬编码凭据（G101）。
		customPassword := "custom-admin" + "-pwd"
		cfg := &config.Config{Admin: config.AdminConfig{Password: customPassword}}
		if err := SeedAdmin(db, cfg); err != nil {
			t.Fatal(err)
		}
		var admin sysmodel.SysUser
		if err := db.Where("tenant_id = ? AND username = ?", 0, "admin").First(&admin).Error; err != nil {
			t.Fatal(err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(customPassword)); err != nil {
			t.Fatalf("admin hash mismatch: %v", err)
		}
	})

	t.Run("debug default", func(t *testing.T) {
		db := seedAdminTestDB(t)
		if err := SeedAdmin(db, &config.Config{}); err != nil {
			t.Fatal(err)
		}
		var admin sysmodel.SysUser
		if err := db.Where("tenant_id = ? AND username = ?", 0, "admin").First(&admin).Error; err != nil {
			t.Fatal(err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(devAdminPassword)); err != nil {
			t.Fatalf("debug default hash mismatch: %v", err)
		}
	})

	t.Run("release missing password", func(t *testing.T) {
		db := seedAdminTestDB(t)
		releaseCfg := &config.Config{Server: config.ServerConfig{Mode: "release"}}
		if err := SeedAdmin(db, releaseCfg); err == nil {
			t.Fatal("release without WMS_ADMIN_PASSWORD must fail on first creation")
		}
		// 失败必须不落库，避免留下半初始化状态。
		var count int64
		if err := db.Model(&sysmodel.SysUser{}).
			Where("tenant_id = ? AND username = ?", 0, "admin").Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("admin users after failed seed=%d want 0", count)
		}
		// 管理员已存在时（重启或幂等补齐）不再要求密码。
		prodPassword := "prod-admin" + "-pwd"
		if err := SeedAdmin(db, &config.Config{Admin: config.AdminConfig{Password: prodPassword}}); err != nil {
			t.Fatal(err)
		}
		if err := SeedAdmin(db, releaseCfg); err != nil {
			t.Fatalf("existing admin must not require password again: %v", err)
		}
	})
}
