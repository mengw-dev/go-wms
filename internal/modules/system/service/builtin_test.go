package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func TestBuiltinAdminProtection(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.SysUser{}, &model.SysRole{}, &model.SysUserRole{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SysUser{
		Base: model.Base{ID: 1}, TenantID: 0, Username: "admin", PasswordHash: "unused", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SysRole{Base: model.Base{ID: 1}, TenantID: 0, Name: "admin", Perms: "*"}).Error; err != nil {
		t.Fatal(err)
	}
	s := New(repository.New(db), "unused", 1)
	if err := s.ResetPassword(context.Background(), 1, &dto.ResetPwdReq{Password: "anything"}); !errors.Is(err, errcode.ModifyAdminForbidden) {
		t.Fatalf("ResetPassword(1): %v", err)
	}
	if err := s.UpdateUser(context.Background(), 1, &dto.UserUpdateReq{}); !errors.Is(err, errcode.ModifyAdminForbidden) {
		t.Fatalf("UpdateUser(1): got %v, want %v", err, errcode.ModifyAdminForbidden)
	}
	if err := s.DeleteUser(context.Background(), 1); !errors.Is(err, errcode.ModifyAdminForbidden) {
		t.Fatalf("DeleteUser(1): got %v, want %v", err, errcode.ModifyAdminForbidden)
	}
	if err := s.UpdateRole(context.Background(), 1, &dto.RoleUpdateReq{}); !errors.Is(err, errcode.ModifyBuiltinRoleForbidden) {
		t.Fatalf("UpdateRole(1): got %v, want %v", err, errcode.ModifyBuiltinRoleForbidden)
	}
	if err := s.DeleteRole(context.Background(), 1); !errors.Is(err, errcode.ModifyBuiltinRoleForbidden) {
		t.Fatalf("DeleteRole(1): got %v, want %v", err, errcode.ModifyBuiltinRoleForbidden)
	}
}
