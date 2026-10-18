package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
)

func TestPermissionLoadCannotRefillAfterRevocation(t *testing.T) {
	s, db, ctx := userFixture(t)
	loaded, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	defer unblock.Do(func() { close(release) })
	if err := db.Callback().Query().After("gorm:query").Register("test:pause_permissions", func(q *gorm.DB) {
		if q.Statement.Table == "sys_role" && strings.Contains(q.Statement.SQL.String(), "perms") {
			once.Do(func() {
				close(loaded)
				select {
				case <-release:
				case <-q.Statement.Context.Done():
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- s.HasPerm(queryCtx, 42, "own") }()
	select {
	case <-loaded:
	case <-queryCtx.Done():
		t.Fatal("permission query did not reach barrier")
	}
	if err := s.UpdateRole(ctx, 10, &dto.RoleUpdateReq{Name: "Own", Perms: ""}); err != nil {
		t.Fatal(err)
	}
	unblock.Do(func() { close(release) })
	select {
	case allowed := <-done:
		if allowed {
			t.Fatal("revoked in-flight permission was accepted")
		}
	case <-queryCtx.Done():
		t.Fatal("permission query did not complete")
	}
	if s.HasPerm(ctx, 42, "own") {
		t.Fatal("old permissions refilled the cache")
	}
}

func TestLoginAliasesShareStableAccountLimit(t *testing.T) {
	s, db, _ := userFixture(t)
	hash, err := hashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.repo.UpdatePassword(context.Background(), 42, hash); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.SysUser{Base: model.Base{ID: 43}, TenantID: 22, Username: "tester", PasswordHash: hash, Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	tenantID, otherTenant := int64(11), int64(22)
	for _, name := range []string{"tester", "TESTER", "téstér", "TÉSTER", "tester"} {
		if _, err := s.Login(context.Background(), &dto.LoginReq{Username: name, Password: "wrong", TenantID: &tenantID}, "alias-client"); !errors.Is(err, errcode.UserOrPwdWrong) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"tester", "téstér"} {
		if _, err := s.Login(context.Background(), &dto.LoginReq{Username: name, Password: "correct-password", TenantID: &tenantID}, "alias-client"); !errors.Is(err, errcode.TooManyLoginAttempts) {
			t.Fatalf("alias bypassed limit: %v", err)
		}
	}
	if _, err := s.Login(context.Background(), &dto.LoginReq{Username: "tester", Password: "correct-password", TenantID: &otherTenant}, "alias-client"); err != nil {
		t.Fatalf("other tenant shared account quota: %v", err)
	}
	// 不传租户和显式租户必须命中同一账号额度。
	if err := db.Delete(&model.SysUser{}, 43).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.Login(context.Background(), &dto.LoginReq{Username: "téstér", Password: "wrong"}, "alias-client"); !errors.Is(err, errcode.TooManyLoginAttempts) {
		t.Fatalf("implicit tenant bypassed limit: %v", err)
	}
}

func TestUnknownLoginNamesSharePrelookupIPLimit(t *testing.T) {
	s, _, _ := userFixture(t)
	for i := range maxLoginIPAttempts {
		_, err := s.Login(context.Background(), &dto.LoginReq{Username: fmt.Sprintf("missing-%d", i), Password: "wrong"}, "unknown-client")
		if !errors.Is(err, errcode.UserOrPwdWrong) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	_, err := s.Login(context.Background(), &dto.LoginReq{Username: "another-missing", Password: "wrong"}, "unknown-client")
	if !errors.Is(err, errcode.TooManyLoginAttempts) {
		t.Fatalf("unknown names bypassed IP cap: %v", err)
	}
}

func TestPasswordByteLimitAcrossAllSetters(t *testing.T) {
	s, db, ctx := userFixture(t)
	valid := strings.Repeat("密", 24) // 72 bytes; within existing 32-character limit.
	hash, err := hashPassword(valid)
	if err != nil || !checkPassword(hash, valid) {
		t.Fatalf("72-byte boundary rejected: %v", err)
	}
	if err := s.repo.UpdatePassword(ctx, 42, hash); err != nil {
		t.Fatal(err)
	}
	var before model.SysUser
	if err := db.First(&before, 42).Error; err != nil {
		t.Fatal(err)
	}
	invalid := strings.Repeat("密", 25)
	for _, action := range []func() error{
		func() error { return s.CreateUser(ctx, &dto.UserCreateReq{Username: "too-long", Password: invalid}) },
		func() error { return s.ResetPassword(ctx, 42, &dto.ResetPwdReq{Password: invalid}) },
		func() error {
			return s.ChangePassword(ctx, 42, &dto.ChangePwdReq{OldPassword: valid, NewPassword: invalid})
		},
	} {
		if err := action(); !errors.Is(err, errcode.PasswordTooLong) {
			t.Fatalf("expected explicit parameter error: %v", err)
		}
	}
	var after model.SysUser
	if err := db.First(&after, 42).Error; err != nil {
		t.Fatal(err)
	}
	if after.PasswordHash != before.PasswordHash || after.TokenVersion != before.TokenVersion {
		t.Fatal("rejected password mutated credentials")
	}
	var count int64
	if err := db.Model(&model.SysUser{}).Where("username = ?", "too-long").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("rejected user persisted: %d %v", count, err)
	}
}
