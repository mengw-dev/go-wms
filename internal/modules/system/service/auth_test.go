package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
)

func TestLoginSelectsTenantWithoutGuessing(t *testing.T) {
	s, db, _ := userFixture(t)
	hash, err := hashPassword("first-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.repo.UpdatePassword(context.Background(), 42, hash); err != nil {
		t.Fatal(err)
	}
	otherHash, err := hashPassword("second-password")
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []*model.SysUser{
		{Base: model.Base{ID: 43}, TenantID: 22, Username: "tester", PasswordHash: otherHash, Status: 1},
		{Base: model.Base{ID: 44}, TenantID: 0, Username: "tester", PasswordHash: hash, Status: 1},
		{Base: model.Base{ID: 45}, TenantID: 22, Username: "unique-name", PasswordHash: hash, Status: 1},
		{Base: model.Base{ID: 46}, TenantID: 22, Username: "disabled", PasswordHash: hash, Status: 1},
	} {
		if err := db.Create(u).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.SysUser{}).Where("id = ?", 46).Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	platform, first, second, missing, invalid := int64(0), int64(11), int64(22), int64(33), int64(-1)
	for i, tt := range []struct {
		name       string
		username   string
		password   string
		tenantID   *int64
		wantID     int64
		wantTenant int64
		wantErr    error
	}{
		{"ambiguous even with correct password", "tester", "first-password", nil, 0, 0, errcode.UserOrPwdWrong},
		{"case insensitive ambiguity", "TESTER", "second-password", nil, 0, 0, errcode.UserOrPwdWrong},
		{"first tenant", "tester", "first-password", &first, 42, 11, nil},
		{"second tenant", "tester", "second-password", &second, 43, 22, nil},
		{"platform means exactly zero", "tester", "first-password", &platform, 44, 0, nil},
		{"unique legacy login", "unique-name", "first-password", nil, 45, 22, nil},
		{"platform cannot find positive tenant", "unique-name", "first-password", &platform, 0, 0, errcode.UserOrPwdWrong},
		{"missing tenant", "tester", "first-password", &missing, 0, 0, errcode.UserOrPwdWrong},
		{"other tenant password", "tester", "second-password", &first, 0, 0, errcode.UserOrPwdWrong},
		{"negative tenant", "tester", "first-password", &invalid, 0, 0, errcode.ParamError},
		{"disabled with wrong password", "disabled", "wrong-password", nil, 0, 0, errcode.UserOrPwdWrong},
		{"disabled with correct password", "disabled", "first-password", nil, 0, 0, errcode.UserDisabled},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := s.Login(context.Background(), &dto.LoginReq{
				Username: tt.username, Password: tt.password, TenantID: tt.tenantID,
			}, fmt.Sprintf("test-client-%d", i))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v want=%v", err, tt.wantErr)
			}
			if tt.wantErr != nil {
				if result != nil {
					t.Fatal("failed login returned a token")
				}
				return
			}
			claims, err := jwt.Parse(s.jwtSecret, result.Token)
			if err != nil {
				t.Fatal(err)
			}
			if claims.UserID != tt.wantID || claims.TenantID != tt.wantTenant || result.UserID != tt.wantID {
				t.Fatalf("wrong login identity: claims=%+v user=%d", claims, result.UserID)
			}
			if result.UserID == 42 && (len(result.Perms) != 1 || result.Perms[0] != "own") {
				t.Fatalf("wrong tenant permissions: %v", result.Perms)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Login(ctx, &dto.LoginReq{Username: "unique-name", Password: "first-password"}, "canceled-client"); !errors.Is(err, context.Canceled) {
		t.Fatalf("query error was hidden: %v", err)
	}
}

func TestCreateUserChecksUsernameWithinTargetTenant(t *testing.T) {
	s, _, ctx := userFixture(t)
	req := &dto.UserCreateReq{Username: "tester", Password: "new-password"}
	if err := s.CreateUser(ctx, req); !errors.Is(err, errcode.UserExist) {
		t.Fatalf("same tenant duplicate: %v", err)
	}
	if err := s.CreateUser(context.Background(), req); err != nil {
		t.Fatalf("another tenant should allow the same username: %v", err)
	}
	u, err := s.repo.GetUserByUsername(context.Background(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	if u.TenantID != 0 {
		t.Fatalf("platform lookup escaped its tenant: %+v", u)
	}
}
