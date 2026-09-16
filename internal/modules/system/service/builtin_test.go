package service

import (
	"context"
	"testing"

	"gowms/internal/modules/system/dto"
	"gowms/internal/pkg/errcode"
)

func TestBuiltinAdminProtection(t *testing.T) {
	s := &Service{}
	if err := s.UpdateUser(context.Background(), 1, &dto.UserUpdateReq{}); err != errcode.ModifyAdminForbidden {
		t.Fatalf("UpdateUser(1): got %v, want %v", err, errcode.ModifyAdminForbidden)
	}
	if err := s.DeleteUser(context.Background(), 1); err != errcode.ModifyAdminForbidden {
		t.Fatalf("DeleteUser(1): got %v, want %v", err, errcode.ModifyAdminForbidden)
	}
	if err := s.UpdateRole(context.Background(), 1, &dto.RoleUpdateReq{}); err != errcode.ModifyBuiltinRoleForbidden {
		t.Fatalf("UpdateRole(1): got %v, want %v", err, errcode.ModifyBuiltinRoleForbidden)
	}
	if err := s.DeleteRole(context.Background(), 1); err != errcode.ModifyBuiltinRoleForbidden {
		t.Fatalf("DeleteRole(1): got %v, want %v", err, errcode.ModifyBuiltinRoleForbidden)
	}
}
