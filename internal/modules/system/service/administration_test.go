package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// Existing mutation tests now supply an actual authorized actor. This fixture
// is separate from the user under test so disabling that user does not disable
// the test's administrator, and it grants no production bypass.
func newAdminActor(t *testing.T, s *Service, ctx context.Context) int64 {
	t.Helper()
	tid := tenant.FromContext(ctx)
	exact := tenant.WithExactTenant(ctx, tid)
	perms := "*"
	if tid != 0 {
		list := []string{"own", "unused"}
		for p := range allowedTenantPerms {
			list = append(list, p)
		}
		sort.Strings(list)
		perms = strings.Join(list, ",")
	}
	id := int64(900000) + tid
	if err := s.repo.CreateRole(exact, &model.SysRole{Base: model.Base{ID: id}, Name: "test-administrator", Perms: perms}); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.CreateUser(exact, &model.SysUser{Base: model.Base{ID: id}, Username: fmt.Sprintf("test-administrator-%d", tid), PasswordHash: "unused", Status: 1}, []int64{id}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestAdministrationChecksRevocationAfterWaitingForRole(t *testing.T) {
	s, db, base := userFixture(t)
	actor := newAdminActor(t, s, base)
	ctx, cancel := context.WithTimeout(base, 5*time.Second)
	defer cancel()
	if !s.HasPerm(ctx, actor, PermSystemUser) {
		t.Fatal("actor lacks initial permission")
	}
	blocker := db.WithContext(ctx).Begin()
	if blocker.Error != nil {
		t.Fatal(blocker.Error)
	}
	defer blocker.Rollback()
	if err := blocker.Model(&model.SysRole{}).Where("id = ?", actor).Update("perms", PermBasic).Error; err != nil {
		t.Fatal(err)
	}
	arrived := make(chan struct{}, 1)
	if err := db.Callback().Query().Before("gorm:query").Register("test:administration-role-wait", func(q *gorm.DB) {
		if q.Statement.Table != "sys_role" {
			return
		}
		if _, ok := q.Statement.Clauses["FOR"].Expression.(clause.Locking); !ok {
			return
		}
		select {
		case arrived <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove("test:administration-role-wait") })
	done, finished := make(chan error, 1), make(chan struct{})
	nickname := "must not persist"
	go func() {
		defer close(finished)
		done <- s.UpdateUser(ctx, actor, 42, &dto.UserUpdateReq{Nickname: &nickname})
	}()
	defer func() { _ = blocker.Rollback(); cancel(); <-finished }()
	select {
	case <-arrived:
	case <-ctx.Done():
		t.Fatal("command did not reach role lock")
	}
	if err := blocker.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, errcode.Forbidden) {
		t.Fatalf("stale authority accepted: %v", err)
	}
	var user model.SysUser
	if err := db.WithContext(ctx).First(&user, 42).Error; err != nil {
		t.Fatal(err)
	}
	if user.Nickname != "Original" {
		t.Fatalf("revoked command persisted: %+v", user)
	}
}
