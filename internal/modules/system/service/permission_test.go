package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func TestTokenTenantAndExpiredPermissionCache(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.SysUser{}, &model.SysRole{}, &model.SysUserRole{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&model.SysUser{Base: model.Base{ID: 42}, TenantID: 11, Username: "test", PasswordHash: "unused", Status: 1, TokenVersion: 1},
		&model.SysRole{Base: model.Base{ID: 10}, TenantID: 11, Name: "role", Perms: "wms:test"},
		&model.SysUserRole{TenantID: 11, UserID: 42, RoleID: 10},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := New(repository.New(db), "unused", 1)
	ctx := tenant.WithTenant(context.Background(), 11)
	if err := s.ValidateToken(ctx, 42, 1); err != nil {
		t.Fatal(err)
	}
	for _, tenantID := range []int64{0, 22} {
		if err := s.ValidateToken(tenant.WithTenant(context.Background(), tenantID), 42, 1); !errors.Is(err, errcode.Unauthorized) {
			t.Fatalf("tenant=%d err=%v", tenantID, err)
		}
	}
	if !s.HasPerm(ctx, 42, "wms:test") {
		t.Fatal("valid permission rejected")
	}
	item := s.permCache[42]
	item.expire = time.Now().Add(-time.Second)
	s.permCache[42] = item
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	if s.HasPerm(ctx, 42, "wms:test") {
		t.Fatal("database failure reused expired permission")
	}
	if err := s.ValidateToken(ctx, 42, 1); err == nil || errors.Is(err, errcode.Unauthorized) {
		t.Fatalf("database error was hidden: %v", err)
	}
}

func TestHasPermDoesNotTrustUserIDOne(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.SysUser{}, &model.SysRole{}, &model.SysUserRole{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&model.SysUser{Base: model.Base{ID: 1}, TenantID: 11, Username: "ordinary-id-one", PasswordHash: "unused", Status: 1},
		&model.SysUser{Base: model.Base{ID: 2}, TenantID: 0, Username: "admin", PasswordHash: "unused", Status: 1},
		&model.SysRole{Base: model.Base{ID: 1}, TenantID: 0, Name: "admin", Perms: "*"},
		&model.SysUserRole{TenantID: 0, UserID: 2, RoleID: 1},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := New(repository.New(db), "unused", 1)
	if s.HasPerm(tenant.WithTenant(context.Background(), 11), 1, "wms:test") {
		t.Fatal("ordinary tenant user with ID 1 was treated as super admin")
	}
	if !s.HasPerm(context.Background(), 2, "anything") {
		t.Fatal("explicit platform admin role lost its wildcard permission")
	}
}
