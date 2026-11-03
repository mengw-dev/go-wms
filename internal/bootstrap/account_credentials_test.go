package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	sysmodel "gowms/internal/modules/system/model"
	sysrepo "gowms/internal/modules/system/repository"
	syssvc "gowms/internal/modules/system/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

func TestManagedAccountCredentialLifecycle(t *testing.T) {
	for _, kind := range []string{"demo", "personal"} {
		t.Run(kind, func(t *testing.T) {
			db := newPersonalTestDB(t)
			seed := func(enabled bool, n int, password string) {
				t.Helper()
				cfg := &config.Config{
					Demo:     config.DemoConfig{Enabled: enabled, Instances: n, Password: password},
					Personal: config.PersonalConfig{Enabled: enabled, Instances: n, Password: password},
				}
				var err error
				if kind == "demo" {
					err = SeedDemoAccounts(db, cfg)
				} else {
					err = SeedPersonalAccounts(db, cfg)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			read := func(index int) sysmodel.SysUser {
				t.Helper()
				cfg := config.Config{}
				tid := cfg.Demo.AccountTenantID(index)
				if kind == "personal" {
					tid = cfg.Personal.AccountTenantID(index)
				}
				var user sysmodel.SysUser
				if err := db.Where("tenant_id = ?", tid).First(&user).Error; err != nil {
					t.Fatal(err)
				}
				return user
			}
			service := syssvc.New(sysrepo.New(db), "test-secret", 1)
			invalid := func(user sysmodel.SysUser) {
				t.Helper()
				if err := service.ValidateToken(tenant.WithTenant(context.Background(), user.TenantID), user.ID, user.TokenVersion); !errors.Is(err, errcode.Unauthorized) {
					t.Fatalf("old credential revived: %v", err)
				}
			}
			seed(true, 2, "initial-password")
			first, second := read(1), read(2)
			seed(true, 2, "initial-password")
			stable := read(1)
			if stable.TokenVersion != first.TokenVersion || stable.PasswordHash != first.PasswordHash {
				t.Fatal("unchanged restart rewrites credentials")
			}
			seed(false, 2, "initial-password")
			disabled := read(1)
			if disabled.Status != 0 || disabled.TokenVersion != first.TokenVersion+1 {
				t.Fatal("disable failed to revoke credentials")
			}
			invalid(first)
			seed(false, 2, "initial-password")
			if read(1).TokenVersion != disabled.TokenVersion {
				t.Fatal("repeated disable changes version")
			}
			seed(true, 2, "initial-password")
			enabled := read(1)
			if enabled.Status != 1 || enabled.TokenVersion != disabled.TokenVersion+1 {
				t.Fatal("reenable did not record credential change")
			}
			invalid(first)
			seed(true, 2, "replacement-password")
			changed := read(1)
			if changed.TokenVersion != enabled.TokenVersion+1 || bcrypt.CompareHashAndPassword([]byte(changed.PasswordHash), []byte("replacement-password")) != nil {
				t.Fatal("password sync did not revoke credentials")
			}
			invalid(enabled)
			seed(true, 1, "replacement-password")
			if read(1).TokenVersion != changed.TokenVersion || read(2).Status != 0 {
				t.Fatal("shrink changed retained account or kept surplus enabled")
			}
			invalid(second)
			seed(true, 2, "replacement-password")
			invalid(second)
			if err := service.ValidateToken(tenant.WithTenant(context.Background(), changed.TenantID), changed.ID, changed.TokenVersion); err != nil {
				t.Fatalf("retained account logged out: %v", err)
			}
			// Configuration remains authoritative for public accounts, including
			// a password changed by a visitor between restarts.
			visitorHash, err := bcrypt.GenerateFromPassword([]byte("visitor-password"), bcrypt.MinCost)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&sysmodel.SysUser{}).Where("id = ?", changed.ID).Updates(map[string]any{"password_hash": string(visitorHash), "token_version": gorm.Expr("token_version + 1")}).Error; err != nil {
				t.Fatal(err)
			}
			visitor := read(1)
			seed(true, 2, "replacement-password")
			invalid(visitor)
			if bcrypt.CompareHashAndPassword([]byte(read(1).PasswordHash), []byte("replacement-password")) != nil {
				t.Fatal("configured password was not restored")
			}
		})
	}
}

func TestManagedAccountSyncRechecksConcurrentCredentials(t *testing.T) {
	db := seedAdminTestDB(t)
	oldHash, err := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	configuredHash, err := bcrypt.GenerateFromPassword([]byte("configured-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := sysmodel.SysUser{TenantID: 10001, Username: "demo1", PasswordHash: string(oldHash), Nickname: "configured", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	type syncKey struct{}
	ready, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	if err := db.Callback().Update().Before("gorm:update").Register("test:seed-credential-write", func(q *gorm.DB) {
		if q.Statement.Context.Value(syncKey{}) == true {
			once.Do(func() {
				close(ready)
				select {
				case <-release:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove("test:seed-credential-write") })
	done := make(chan error, 1)
	go func() {
		defer close(finished)
		done <- syncManagedAccount(db.WithContext(context.WithValue(ctx, syncKey{}, true)), &user, "configured-password", string(configuredHash), "configured")
	}()
	defer func() { unblock.Do(func() { close(release) }); cancel(); <-finished }()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("sync did not reach write barrier")
	}
	// Another process has already applied the same configuration, revoking the
	// old credential. The waiting process must preserve that newer generation.
	if err := db.WithContext(ctx).Model(&sysmodel.SysUser{}).Where("id = ?", user.ID).
		Updates(map[string]any{"password_hash": string(configuredHash), "token_version": gorm.Expr("token_version + 7")}).Error; err != nil {
		t.Fatal(err)
	}
	unblock.Do(func() { close(release) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	var current sysmodel.SysUser
	if err := db.First(&current, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.TokenVersion != 8 || current.PasswordHash != string(configuredHash) {
		t.Fatalf("sync overwrote newer credentials: version=%d", current.TokenVersion)
	}
}
