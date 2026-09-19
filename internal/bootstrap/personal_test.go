package bootstrap

import (
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

// 集成测试：需要本地 MySQL；连不上自动跳过（与项目其他集成测试一致）。
func newPersonalTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn,
		&sysmodel.SysUser{}, &sysmodel.SysRole{}, &sysmodel.SysUserRole{},
		&model.Warehouse{}, &model.Location{}, &model.SKU{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{},
		&taskmodel.Task{},
		&inboundmodel.ReceiptOrder{}, &inboundmodel.ReceiptOrderDetail{}, &inboundmodel.ImportTask{},
		&outboundmodel.ShipmentOrder{}, &outboundmodel.ShipmentOrderDetail{}, &outboundmodel.Allocation{},
		&stocktakemodel.StocktakeOrder{}, &stocktakemodel.StocktakeDetail{},
	)
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatalf("register tenant callbacks: %v", err)
	}
	return db
}

func personalCfg(instances int) *config.Config {
	return &config.Config{Personal: config.PersonalConfig{
		Enabled: true, Instances: instances, Password: "user123456",
	}}
}

// TestSeedPersonalAccountsIsolatedTenants 持久账号核心约定：
// 一人一租户、幂等创建、初始数据落在自己租户、角色权限不含 wms:demo（否则会触发演示重置）。
func TestSeedPersonalAccountsIsolatedTenants(t *testing.T) {
	db := newPersonalTestDB(t)
	cfg := personalCfg(2)
	if err := SeedPersonalAccounts(db, cfg); err != nil {
		t.Fatalf("seed personal accounts: %v", err)
	}
	if err := SeedPersonalAccounts(db, cfg); err != nil { // 幂等：重复执行不产生重复数据
		t.Fatalf("seed personal accounts again: %v", err)
	}

	for i := 1; i <= 2; i++ {
		username := cfg.Personal.AccountUsername(i)
		tenantID := cfg.Personal.AccountTenantID(i)

		var user sysmodel.SysUser
		if err := db.Where("username = ?", username).First(&user).Error; err != nil {
			t.Fatalf("personal user %s not created: %v", username, err)
		}
		if user.TenantID != tenantID || user.Status != 1 {
			t.Fatalf("unexpected personal user %s: %+v", username, user)
		}

		var count int64
		if err := db.Model(&sysmodel.SysUser{}).Where("username = ?", username).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("idempotent seed expected exactly 1 user %s, got %d (err=%v)", username, count, err)
		}

		var role sysmodel.SysRole
		if err := db.Where("tenant_id = ? AND name = ?", tenantID, "personal").First(&role).Error; err != nil {
			t.Fatalf("personal role of tenant %d not created: %v", tenantID, err)
		}
		if strings.Contains(role.Perms, "wms:demo") {
			t.Fatalf("personal role must not include wms:demo (would trigger demo reset): %s", role.Perms)
		}

		var warehouses int64
		if err := db.Model(&model.Warehouse{}).Where("tenant_id = ?", tenantID).Count(&warehouses).Error; err != nil || warehouses == 0 {
			t.Fatalf("tenant %d should be seeded with initial data, got %d warehouses (err=%v)", tenantID, warehouses, err)
		}
	}
}

// TestSeedPersonalAccountsShrinkAndDisable 缩容/关闭只禁用账号，不动已保留的数据。
func TestSeedPersonalAccountsShrinkAndDisable(t *testing.T) {
	db := newPersonalTestDB(t)
	if err := SeedPersonalAccounts(db, personalCfg(2)); err != nil {
		t.Fatalf("seed personal accounts: %v", err)
	}

	// 缩容到 1：user2 被禁用，其租户数据保留
	if err := SeedPersonalAccounts(db, personalCfg(1)); err != nil {
		t.Fatalf("shrink personal accounts: %v", err)
	}
	var user1, user2 sysmodel.SysUser
	if err := db.Where("username = ?", "user1").First(&user1).Error; err != nil || user1.Status != 1 {
		t.Fatalf("user1 should stay enabled, got %+v (err=%v)", user1, err)
	}
	if err := db.Where("username = ?", "user2").First(&user2).Error; err != nil || user2.Status != 0 {
		t.Fatalf("surplus user2 should be disabled, got %+v (err=%v)", user2, err)
	}
	var kept int64
	if err := db.Model(&model.Warehouse{}).
		Where("tenant_id = ?", personalCfg(2).Personal.AccountTenantID(2)).Count(&kept).Error; err != nil || kept == 0 {
		t.Fatalf("disabled account data must be kept, got %d warehouses (err=%v)", kept, err)
	}

	// 关闭开关：全部禁用
	off := &config.Config{Personal: config.PersonalConfig{Enabled: false, Instances: 1}}
	if err := SeedPersonalAccounts(db, off); err != nil {
		t.Fatalf("disable personal accounts: %v", err)
	}
	var enabled int64
	if err := db.Model(&sysmodel.SysUser{}).
		Where("username REGEXP ? AND status = 1", "^user[0-9]+$").Count(&enabled).Error; err != nil || enabled != 0 {
		t.Fatalf("all personal accounts should be disabled, got %d enabled (err=%v)", enabled, err)
	}
}

// TestSeedPersonalAccountsDoesNotTouchDemoAccounts 个人账号种子不能误伤演示账号（命名空间隔离）。
func TestSeedPersonalAccountsDoesNotTouchDemoAccounts(t *testing.T) {
	db := newPersonalTestDB(t)
	demoCfg := &config.Config{Demo: config.DemoConfig{Enabled: true, Instances: 1, Password: "demo123456"}}
	if err := SeedDemoAccounts(db, demoCfg); err != nil {
		t.Fatalf("seed demo accounts: %v", err)
	}
	if err := SeedPersonalAccounts(db, personalCfg(1)); err != nil {
		t.Fatalf("seed personal accounts: %v", err)
	}

	var demo sysmodel.SysUser
	if err := db.Where("username = ?", "demo1").First(&demo).Error; err != nil || demo.Status != 1 {
		t.Fatalf("demo1 should stay enabled, got %+v (err=%v)", demo, err)
	}
	if demo.TenantID != demoCfg.Demo.AccountTenantID(1) {
		t.Fatalf("demo1 tenant changed unexpectedly: %+v", demo)
	}

	// 反向：关闭个人账号时不能禁用演示账号
	off := &config.Config{Personal: config.PersonalConfig{Enabled: false, Instances: 1}}
	if err := SeedPersonalAccounts(db, off); err != nil {
		t.Fatalf("disable personal accounts: %v", err)
	}
	if err := db.Where("username = ?", "demo1").First(&demo).Error; err != nil || demo.Status != 1 {
		t.Fatalf("disabling personal accounts must not disable demo1, got %+v (err=%v)", demo, err)
	}
}
