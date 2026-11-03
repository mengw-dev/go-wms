package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/tenant"
)

func TestUserEditorCannotRestoreRevokedAccountState(t *testing.T) {
	s := newBusinessTenantStack(t)
	for _, row := range []struct {
		id, tid         int64
		username, perms string
	}{
		{1, 0, "admin", "*"}, {42, 11, "target", "wms:basic"},
	} {
		ctx := tenant.WithExactTenant(context.Background(), row.tid)
		for _, model := range []any{
			&sysmodel.SysUser{Base: sysmodel.Base{ID: row.id}, Username: row.username, PasswordHash: "unchanged", Nickname: "original", Status: 1},
			&sysmodel.SysRole{Base: sysmodel.Base{ID: row.id}, Name: row.username, Perms: row.perms},
			&sysmodel.SysUserRole{UserID: row.id, RoleID: row.id},
		} {
			requireBusinessError(t, s.db.WithContext(ctx).Create(model).Error, nil)
		}
	}
	cfg := &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("u", 32)}}
	a := New(cfg, s.db, nil, nil)
	router, err := a.NewRouter()
	requireBusinessError(t, err, nil)
	token, err := jwt.Generate(cfg.JWT.Secret, time.Hour, 1, "admin", 1, 0)
	requireBusinessError(t, err, nil)
	request := func(method, path, body string, wantHTTP, wantCode int) json.RawMessage {
		t.Helper()
		req := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var response struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		requireBusinessError(t, json.Unmarshal(w.Body.Bytes(), &response), nil)
		if w.Code != wantHTTP || response.Code != wantCode {
			t.Fatalf("%s %s = %d %s, want %d/%d", method, path, w.Code, w.Body.String(), wantHTTP, wantCode)
		}
		return response.Data
	}
	request("PUT", "/system/users/42", `{"expected_version":1,"status":0,"role_ids":[]}`, 200, 0)
	request("PUT", "/system/users/42", `{"expected_version":1,"nickname":"stale","status":1,"role_ids":["42"]}`, 409, 40902)
	var user sysmodel.SysUser
	requireBusinessError(t, s.db.First(&user, 42).Error, nil)
	var links int64
	requireBusinessError(t, s.db.Model(&sysmodel.SysUserRole{}).Where("user_id = 42").Count(&links).Error, nil)
	if user.Status != 0 || user.Nickname != "original" || user.TokenVersion != 2 || links != 0 {
		t.Fatal("stale editor restored account or role")
	}
	var page struct {
		List []struct {
			Version int `json:"version"`
		} `json:"list"`
	}
	requireBusinessError(t, json.Unmarshal(request("GET", "/system/users?keyword=target", "", 200, 0), &page), nil)
	if len(page.List) != 1 || page.List[0].Version != 2 {
		t.Fatal("user list lost editing version")
	}
	request("PUT", "/system/users/42", `{"expected_version":2,"nickname":"fresh","status":0,"role_ids":[]}`, 200, 0)
	requireBusinessError(t, s.db.First(&user, 42).Error, nil)
	if user.TokenVersion != 2 || user.Nickname != "fresh" || user.Status != 0 {
		t.Fatal("unchanged security state revoked credential")
	}
	request("PUT", "/system/users/42", `{"nickname":"missing version"}`, 400, 400)
	request("PUT", "/system/users/42/password", `{"password":"replacement-password"}`, 200, 0)
	request("PUT", "/system/users/42", `{"expected_version":3,"nickname":"before reset"}`, 409, 40902)
	request("PUT", "/system/users/42/status", `{"status":1}`, 200, 0)
	request("PUT", "/system/users/42/status", `{"status":1}`, 200, 0)
	request("PUT", "/system/users/42", `{"expected_version":5,"nickname":"still signed in","status":1}`, 200, 0)
	requireBusinessError(t, s.db.First(&user, 42).Error, nil)
	if user.Version != 6 || user.TokenVersion != 4 || user.Status != 1 {
		t.Fatalf("editor revision and credential revocation were conflated: %+v", user)
	}
	token, err = jwt.Generate(cfg.JWT.Secret, time.Hour, 42, "target", 4, 11)
	requireBusinessError(t, err, nil)
	request("GET", "/profile", "", 200, 0)
}
