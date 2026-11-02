package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/tenant"
)

func TestAdministrationDelegationHTTP(t *testing.T) {
	s := newBusinessTenantStack(t)
	for _, row := range []struct {
		id, tid     int64
		name, perms string
	}{
		{1, 0, "admin", "*"},
		{10, 0, "user-manager", "wms:system:user,wms:basic"},
		{20, 0, "role-manager", "wms:system:role,wms:basic"},
		{30, 0, "higher", "wms:inventory"},
		{40, 0, "basic", "wms:basic"},
		{110, 11, "tenant-manager", "wms:system:user,wms:system:role,wms:basic"},
		{130, 11, "higher", "wms:inventory"},
		{140, 11, "basic", "wms:basic"},
	} {
		ctx := tenant.WithExactTenant(context.Background(), row.tid)
		for _, model := range []any{
			&sysmodel.SysRole{Base: sysmodel.Base{ID: row.id}, Name: row.name, Perms: row.perms},
			&sysmodel.SysUser{Base: sysmodel.Base{ID: row.id}, Username: row.name, PasswordHash: "unchanged", Nickname: "original", Status: 1},
			&sysmodel.SysUserRole{UserID: row.id, RoleID: row.id},
		} {
			requireBusinessError(t, s.db.WithContext(ctx).Create(model).Error, nil)
		}
	}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("a", 32)}}
	a := New(cfg, s.db, nil, nil)
	router, err := a.NewRouter()
	requireBusinessError(t, err, nil)
	request := func(actor, tid int64, method, path, body string, wantCode int) json.RawMessage {
		t.Helper()
		token, err := jwt.Generate(cfg.JWT.Secret, time.Hour, actor, "actor", 1, tid)
		requireBusinessError(t, err, nil)
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var result struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("%s %s status %d: %s", method, path, w.Code, w.Body.String())
		}
		if result.Code != wantCode {
			t.Fatalf("%s %s: %s; want code %d", method, path, w.Body.String(), wantCode)
		}
		return result.Data
	}
	for _, tc := range []struct {
		name               string
		actor, tid         int64
		method, path, body string
		code               int
	}{
		{"self elevation", 10, 0, "PUT", "/system/users/10", `{"role_ids":["1"]}`, 40300},
		{"create elevated account", 10, 0, "POST", "/system/users", `{"username":"escalated","password":"test-password","role_ids":["1"]}`, 40300},
		{"reset higher account", 10, 0, "PUT", "/system/users/30/password", `{"password":"test-password"}`, 40300},
		{"disable higher account", 10, 0, "PUT", "/system/users/30/status", `{"status":0}`, 40300},
		{"delete higher account", 10, 0, "DELETE", "/system/users/30", "", 40300},
		{"rewrite higher account", 10, 0, "PUT", "/system/users/30", `{"nickname":"changed","role_ids":["40"]}`, 40300},
		{"platform scope is not authority", 10, 0, "PUT", "/system/users/140", `{"nickname":"changed"}`, 40300},
		{"create higher role", 20, 0, "POST", "/system/roles", `{"name":"escalated","perms":"wms:inventory"}`, 40300},
		{"grant self wildcard", 20, 0, "PUT", "/system/roles/20", `{"name":"role-manager","perms":"*"}`, 40300},
		{"rewrite higher role", 20, 0, "PUT", "/system/roles/30", `{"name":"higher","perms":"wms:basic"}`, 40300},
		{"delete higher role", 20, 0, "DELETE", "/system/roles/30", "", 40300},
		{"tenant self elevation", 110, 11, "PUT", "/system/users/110", `{"role_ids":["130"]}`, 40300},
		{"tenant role escalation", 110, 11, "POST", "/system/roles", `{"name":"escalated","perms":"wms:inventory"}`, 40300},
		{"foreign target remains hidden", 110, 11, "PUT", "/system/users/40", `{"nickname":"changed"}`, errcode.UserIDInvalid.Code},
		{"foreign role assignment", 110, 11, "PUT", "/system/users/140", `{"role_ids":["40"]}`, errcode.RoleIDInvalid.Code},
		{"protected account", 1, 0, "PUT", "/system/users/1", `{"nickname":"changed"}`, errcode.ModifyAdminForbidden.Code},
		{"protected role", 1, 0, "PUT", "/system/roles/1", `{"name":"other","perms":""}`, errcode.ModifyBuiltinRoleForbidden.Code},
		{"no user management", 40, 0, "GET", "/system/users/role-options", "", 40300},
	} {
		t.Run(tc.name, func(t *testing.T) { request(tc.actor, tc.tid, tc.method, tc.path, tc.body, tc.code) })
	}
	// Rejections must leave account state, passwords, roles and links unchanged.
	for _, id := range []int64{10, 30, 110, 140} {
		var user sysmodel.SysUser
		requireBusinessError(t, s.db.First(&user, id).Error, nil)
		if user.Status != 1 || user.Nickname != "original" || user.PasswordHash != "unchanged" || user.TokenVersion != 1 {
			t.Fatalf("rejected command changed user: %+v", user)
		}
		var links []sysmodel.SysUserRole
		requireBusinessError(t, s.db.Where("user_id = ?", id).Find(&links).Error, nil)
		if len(links) != 1 || links[0].RoleID != id {
			t.Fatalf("changed role links: %+v", links)
		}
	}
	var rejectedRows int64
	requireBusinessError(t, s.db.Model(&sysmodel.SysUser{}).Where("username = ?", "escalated").Count(&rejectedRows).Error, nil)
	if rejectedRows != 0 {
		t.Fatal("unauthorized user persisted")
	}
	requireBusinessError(t, s.db.Model(&sysmodel.SysRole{}).Where("name = ?", "escalated").Count(&rejectedRows).Error, nil)
	if rejectedRows != 0 {
		t.Fatal("unauthorized role persisted")
	}

	// A user manager needs no role-management permission for legal options.
	request(10, 0, "GET", "/system/roles/all", "", 40300)
	for _, tc := range []struct {
		actor, tid int64
		target     string
		allowed    map[string]bool
	}{
		{10, 0, "", map[string]bool{"10": true, "40": true}},
		{110, 11, "?user_id=140", map[string]bool{"110": true, "140": true}},
		{1, 0, "?user_id=140", map[string]bool{"110": true, "130": true, "140": true}},
	} {
		data := request(tc.actor, tc.tid, "GET", "/system/users/role-options"+tc.target, "", 0)
		var roles []struct {
			ID string `json:"id"`
		}
		requireBusinessError(t, json.Unmarshal(data, &roles), nil)
		if len(roles) != len(tc.allowed) {
			t.Fatalf("role options = %s", data)
		}
		for _, role := range roles {
			if !tc.allowed[role.ID] {
				t.Fatalf("unauthorized option: %s", data)
			}
		}
	}
	request(10, 0, "POST", "/system/users", `{"username":"legal-user","password":"test-password","role_ids":["40"]}`, 0)
	request(20, 0, "POST", "/system/roles", `{"name":"legal-role","perms":"wms:basic"}`, 0)
	request(10, 0, "PUT", "/system/users/40", `{"nickname":"legal edit","role_ids":["40","40"]}`, 0)
	request(1, 0, "PUT", "/system/users/140", `{"nickname":"platform managed","role_ids":["140"]}`, 0)
	// Privileged administration still cannot put wildcard permissions in a tenant role.
	request(1, 0, "PUT", "/system/roles/140", `{"name":"basic","perms":"*"}`, errcode.RolePermNotAllowed.Code)
	// User management can delegate a peer only within its current permission ceiling.
	request(10, 0, "PUT", "/system/users/40", fmt.Sprintf(`{"role_ids":["%d"]}`, 10), 0)
}
