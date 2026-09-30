package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

// 租户角色只能授予注册表中登记过的权限：*、未注册权限与空权限项必须被拒绝，
// 无法通过角色管理自我提权；平台旁路（内置超级管理员）保留 * 语义。
func TestTenantRolePermissionBoundary(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.SysRole{}, &model.SysUser{}, &model.SysUserRole{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	s := New(repository.New(db), "unused", 1)
	ctx := tenant.WithTenant(context.Background(), 11)

	rejected := []struct {
		name  string
		perms string
	}{
		{name: "star", perms: "*"},
		{name: "unknown", perms: "wms:system:role,wms:not-exists"},
		{name: "empty-item", perms: "wms:basic,,wms:inventory"},
		{name: "trailing-comma", perms: "wms:basic,"},
		{name: "demo-only", perms: "wms:demo"},
	}
	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			err := s.CreateRole(ctx, &dto.RoleCreateReq{Name: tc.name, Perms: tc.perms})
			if !errors.Is(err, errcode.RolePermNotAllowed) {
				t.Fatalf("CreateRole(%q): got %v, want %v", tc.perms, err, errcode.RolePermNotAllowed)
			}
			if _, err := s.repo.GetRoleByName(ctx, tc.name); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("rejected role %q must not exist: %v", tc.name, err)
			}
		})
	}

	// 合法权限组合可以创建，更新路径受同样约束且失败时不改动原值。
	if err := s.CreateRole(ctx, &dto.RoleCreateReq{Name: "ops", Perms: "wms:basic,wms:system:role"}); err != nil {
		t.Fatal(err)
	}
	role, err := s.repo.GetRoleByName(ctx, "ops")
	if err != nil {
		t.Fatal(err)
	}
	for _, perms := range []string{"*", "wms:system:role,wms:not-exists"} {
		if err := s.UpdateRole(ctx, role.ID, &dto.RoleUpdateReq{Name: "ops", Perms: perms}); !errors.Is(err, errcode.RolePermNotAllowed) {
			t.Fatalf("UpdateRole(%q): got %v, want %v", perms, err, errcode.RolePermNotAllowed)
		}
	}
	unchanged, err := s.repo.GetRoleByID(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Perms != "wms:basic,wms:system:role" {
		t.Fatalf("rejected update must not change perms: %q", unchanged.Perms)
	}
	if err := s.UpdateRole(ctx, role.ID, &dto.RoleUpdateReq{Name: "ops", Perms: "wms:inbound:view"}); err != nil {
		t.Fatal(err)
	}
	updated, err := s.repo.GetRoleByID(ctx, role.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Perms != "wms:inbound:view" {
		t.Fatalf("perms after update = %q", updated.Perms)
	}

	// 精确租户 0（无平台旁路，如 API Key 入口）同样不能授予 *。
	exact := tenant.WithExactTenant(context.Background(), 0)
	if err := s.CreateRole(exact, &dto.RoleCreateReq{Name: "exact-zero", Perms: "*"}); !errors.Is(err, errcode.RolePermNotAllowed) {
		t.Fatalf("exact tenant 0 with *: got %v, want %v", err, errcode.RolePermNotAllowed)
	}

	// 平台旁路保留超级管理员语义：平台角色由平台管理员维护。
	platform := tenant.WithTenant(context.Background(), 0)
	if err := s.CreateRole(platform, &dto.RoleCreateReq{Name: "platform-ops", Perms: "*"}); err != nil {
		t.Fatal(err)
	}
}
