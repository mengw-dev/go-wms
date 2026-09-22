package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/typex"
	"gowms/internal/testutil"
)

func userFixture(t *testing.T) (*Service, *gorm.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.SysUser{}, &model.SysRole{}, &model.SysUserRole{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	for _, role := range []*model.SysRole{
		{Base: model.Base{ID: 10}, TenantID: 11, Name: "Own", Perms: "own"},
		{Base: model.Base{ID: 20}, TenantID: 22, Name: "Foreign", Perms: "foreign"},
		{Base: model.Base{ID: 30}, TenantID: 11, Name: "Unused", Perms: "unused"},
	} {
		if err := db.Create(role).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx := tenant.WithTenant(context.Background(), 11)
	repo := repository.New(db)
	if err := repo.CreateUser(ctx, &model.SysUser{Base: model.Base{ID: 42}, Username: "tester", Nickname: "Original", PasswordHash: "unused", Status: 1}, []int64{10}); err != nil {
		t.Fatal(err)
	}
	return New(repo, "unused", 1), db, ctx
}

func TestUserPartialUpdateAndTenantRoles(t *testing.T) {
	s, db, ctx := userFixture(t)
	disabled := 0
	if err := s.UpdateUser(ctx, 42, &dto.UserUpdateReq{Status: &disabled}); err != nil {
		t.Fatal(err)
	}
	user, err := s.repo.GetUserByID(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if user.Nickname != "Original" || user.Status != 0 || user.TokenVersion != 2 {
		t.Fatalf("status update changed other fields: %+v", user)
	}
	replacement := "Must roll back"
	if err := s.UpdateUser(ctx, 42, &dto.UserUpdateReq{Nickname: &replacement, RoleIDs: typex.Int64List{20}}); !errors.Is(err, errcode.RoleIDInvalid) {
		t.Fatalf("cross-tenant assignment: %v", err)
	}
	user, err = s.repo.GetUserByID(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if user.Nickname != "Original" {
		t.Fatal("failed role assignment did not roll back nickname")
	}
	if err := s.CreateUser(ctx, &dto.UserCreateReq{Username: "invalid-user", Password: "test-password", RoleIDs: typex.Int64List{20}}); !errors.Is(err, errcode.RoleIDInvalid) {
		t.Fatalf("cross-tenant create: %v", err)
	}
	var count int64
	if err := db.Model(&model.SysUser{}).Where("username = ?", "invalid-user").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("invalid user survived transaction rollback")
	}
	// 平台代操作写入的关联仍属于目标用户的租户，重复角色 ID 不产生重复关联。
	if err := s.UpdateUser(context.Background(), 42, &dto.UserUpdateReq{RoleIDs: typex.Int64List{10, 10}}); err != nil {
		t.Fatal(err)
	}
	var links []model.SysUserRole
	if err := db.Where("user_id = ?", 42).Find(&links).Error; err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].TenantID != 11 {
		t.Fatalf("role links=%+v", links)
	}
	if err := s.DeleteRole(ctx, 10); !errors.Is(err, errcode.RoleInUse) {
		t.Fatalf("delete assigned role: %v", err)
	}
	empty := ""
	if err := s.UpdateUser(ctx, 42, &dto.UserUpdateReq{Nickname: &empty, RoleIDs: typex.Int64List{}}); err != nil {
		t.Fatal(err)
	}
	user, err = s.repo.GetUserByID(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if user.Nickname != "" {
		t.Fatal("explicit empty nickname did not clear value")
	}
	if err := s.DeleteRole(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUser(ctx, 42, &dto.UserUpdateReq{RoleIDs: typex.Int64List{10}}); !errors.Is(err, errcode.RoleIDInvalid) {
		t.Fatalf("deleted role assignment: %v", err)
	}
}

func TestForeignRoleLinksCannotGrantPermissions(t *testing.T) {
	s, db, ctx := userFixture(t)
	// 模拟历史脏数据：用户属于租户 11，关联和角色属于租户 22。
	if err := db.Create(&model.SysUserRole{TenantID: 22, UserID: 42, RoleID: 20}).Error; err != nil {
		t.Fatal(err)
	}
	for _, queryCtx := range []context.Context{ctx, context.Background()} {
		roles, perms, err := s.loadRolesAndPerms(queryCtx, 42)
		if err != nil {
			t.Fatal(err)
		}
		if len(roles) != 1 || roles[0] != "Own" || len(perms) != 1 || perms[0] != "own" {
			t.Fatalf("foreign role leaked: roles=%v perms=%v", roles, perms)
		}
		users, _, err := s.ListUsers(queryCtx, &dto.UserListQuery{Page: 1, PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(users) != 1 || len(users[0].RoleIDs) != 1 || users[0].RoleIDs[0] != 10 {
			t.Fatalf("foreign role ID leaked: %+v", users)
		}
	}
}

func TestRoleDeletionWaitsForConcurrentAssignment(t *testing.T) {
	s, db, ctx := userFixture(t)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	assignment := db.WithContext(ctx).Begin()
	if assignment.Error != nil {
		t.Fatal(assignment.Error)
	}
	defer assignment.Rollback()
	var role model.SysRole
	if err := assignment.Clauses(clause.Locking{Strength: "UPDATE"}).First(&role, 30).Error; err != nil {
		t.Fatal(err)
	}
	if err := assignment.Create(&model.SysUserRole{TenantID: 11, UserID: 42, RoleID: 30}).Error; err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	if err := db.Callback().Query().Before("gorm:query").Register("test:role_delete_lock", func(query *gorm.DB) {
		if query.Statement.Table == "sys_role" {
			select {
			case started <- struct{}{}:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.DeleteRole(ctx, 30) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("delete did not attempt to lock role")
	}
	if err := assignment.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, errcode.RoleInUse) {
			t.Fatalf("concurrent assignment lost its role: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("role deletion did not finish")
	}
	if err := db.First(&role, 30).Error; err != nil {
		t.Fatalf("assigned role was deleted: %v", err)
	}
}
