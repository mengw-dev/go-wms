package service

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

func TestOnlyConfiguredDemoTenantsCanAcquireOrReset(t *testing.T) {
	// 不可达 Redis：错误租户必须在访问 Redis 或数据库之前被拒绝。
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = rdb.Close() })
	s := &Service{cfg: &config.Config{Demo: config.DemoConfig{Enabled: true, Instances: 1}}, rdb: rdb}
	if !s.IsDemoTenant(tenant.WithTenant(context.Background(), 10001)) {
		t.Fatal("configured tenant not recognized")
	}
	for _, id := range []int64{0, 22, 10002, 20001} {
		ctx := tenant.WithTenant(context.Background(), id)
		if s.IsDemoTenant(ctx) {
			t.Fatalf("ordinary tenant %d recognized as demo", id)
		}
		if _, err := s.AcquireSession(ctx); !errors.Is(err, errcode.Forbidden) {
			t.Fatalf("tenant %d acquired demo session: %v", id, err)
		}
		if err := s.resetLocked(ctx, "unused"); !errors.Is(err, errcode.Forbidden) {
			t.Fatalf("tenant %d entered demo reset: %v", id, err)
		}
	}
}

func TestPersonalAccountCredentialsIncludeTenant(t *testing.T) {
	s := &Service{cfg: &config.Config{Personal: config.PersonalConfig{Enabled: true, Instances: 2, Password: "test-password"}}}
	accounts, err := s.PersonalAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 {
		t.Fatalf("accounts=%d", len(accounts))
	}
	for i, account := range accounts {
		if account.TenantID != s.cfg.Personal.AccountTenantID(i+1) || account.Password != "" {
			t.Fatalf("wrong public account listing: %+v", account)
		}
		claimed, err := s.ClaimPersonalAccount(account.Username)
		if err != nil {
			t.Fatal(err)
		}
		if claimed.TenantID != account.TenantID || claimed.Password != "test-password" {
			t.Fatalf("wrong login credentials: %+v", claimed)
		}
	}
}
