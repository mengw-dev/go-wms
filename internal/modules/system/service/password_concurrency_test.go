package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

func TestChangePasswordRejectsStaleCredentialSnapshot(t *testing.T) {
	for _, action := range []string{"administrator reset", "parallel self change", "disable and reenable", "delete"} {
		t.Run(action, func(t *testing.T) {
			s, db, base := userFixture(t)
			actor := newAdminActor(t, s, base)
			hash, err := hashPassword("original-password")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.repo.UpdatePassword(base, 42, hash); err != nil {
				t.Fatal(err)
			}
			before, err := s.repo.GetUserByID(base, 42)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(base, 8*time.Second)
			defer cancel()
			type barrierKey struct{}
			pausedCtx := context.WithValue(ctx, barrierKey{}, true)
			loaded, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var once, unblock sync.Once
			if err := db.Callback().Query().After("gorm:query").Register("test:password-snapshot", func(q *gorm.DB) {
				if q.Statement.Table == "sys_user" && q.Statement.Context.Value(barrierKey{}) == true {
					once.Do(func() {
						close(loaded)
						select {
						case <-release:
						case <-ctx.Done():
						}
					})
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Query().Remove("test:password-snapshot") })
			done := make(chan error, 1)
			go func() {
				defer close(finished)
				done <- s.ChangePassword(pausedCtx, 42, before.TokenVersion, &dto.ChangePwdReq{OldPassword: "original-password", NewPassword: "stale-password"})
			}()
			defer func() { unblock.Do(func() { close(release) }); cancel(); <-finished }()
			select {
			case <-loaded:
			case <-ctx.Done():
				t.Fatal("password read did not reach barrier")
			}
			switch action {
			case "administrator reset":
				err = s.ResetPassword(ctx, actor, 42, &dto.ResetPwdReq{Password: "winning-password"})
			case "parallel self change":
				err = s.ChangePassword(ctx, 42, before.TokenVersion, &dto.ChangePwdReq{OldPassword: "original-password", NewPassword: "winning-password"})
			case "disable and reenable":
				for _, status := range []int{0, 1} {
					if err = s.UpdateUser(ctx, actor, 42, &dto.UserUpdateReq{Status: &status}); err != nil {
						break
					}
				}
			case "delete":
				err = s.DeleteUser(ctx, actor, 42)
			}
			if err != nil {
				t.Fatal(err)
			}
			var winner model.SysUser
			if err := db.Unscoped().First(&winner, 42).Error; err != nil {
				t.Fatal(err)
			}
			unblock.Do(func() { close(release) })
			if err := <-done; !errors.Is(err, errcode.Unauthorized) {
				t.Fatalf("stale password command accepted: %v", err)
			}
			var after model.SysUser
			if err := db.Unscoped().First(&after, 42).Error; err != nil {
				t.Fatal(err)
			}
			if after.PasswordHash != winner.PasswordHash || after.TokenVersion != winner.TokenVersion {
				t.Fatal("stale command overwrote newer credentials")
			}
			if err := s.ValidateToken(base, 42, before.TokenVersion); !errors.Is(err, errcode.Unauthorized) {
				t.Fatalf("old token remains valid: %v", err)
			}
		})
	}
}

func TestChangePasswordRejectsAlreadyRevokedIdentity(t *testing.T) {
	s, _, ctx := userFixture(t)
	hash, err := hashPassword("original-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.repo.UpdatePassword(ctx, 42, hash); err != nil {
		t.Fatal(err)
	}
	u, err := s.repo.GetUserByID(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	req := &dto.ChangePwdReq{OldPassword: "original-password", NewPassword: "new-password"}
	for _, version := range []int{0, u.TokenVersion - 1} {
		if err := s.ChangePassword(ctx, 42, version, req); !errors.Is(err, errcode.Unauthorized) {
			t.Fatalf("accepted stale token before read: %v", err)
		}
	}
	if err := s.ChangePassword(tenant.WithTenant(ctx, 0), 42, u.TokenVersion, req); !errors.Is(err, errcode.Unauthorized) {
		t.Fatalf("platform identity changed foreign password: %v", err)
	}
}
