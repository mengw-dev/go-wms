package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/tenant"
)

func TestSelfPasswordHTTPUsesSignedCredentialVersion(t *testing.T) {
	s := newBusinessTenantStack(t)
	ctx := tenant.WithExactTenant(context.Background(), 11)
	hash, err := bcrypt.GenerateFromPassword([]byte("original-password"), bcrypt.MinCost)
	requireBusinessError(t, err, nil)
	u := &sysmodel.SysUser{Username: "password-owner", PasswordHash: string(hash), Status: 1}
	requireBusinessError(t, s.db.WithContext(ctx).Create(u).Error, nil)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("p", 32)}}
	a := New(cfg, s.db, nil, nil)
	router, err := a.NewRouter()
	requireBusinessError(t, err, nil)
	token, err := jwt.Generate(cfg.JWT.Secret, time.Hour, u.ID, u.Username, u.TokenVersion, 11)
	requireBusinessError(t, err, nil)
	change := func(token, body string, want int) {
		t.Helper()
		req := httptest.NewRequest("PUT", "/api/v1/password", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var result struct {
			Code int `json:"code"`
		}
		requireBusinessError(t, json.Unmarshal(w.Body.Bytes(), &result), nil)
		if result.Code != want {
			t.Fatalf("password result = %s, want %d", w.Body.String(), want)
		}
	}
	change(token, `{"old_password":"wrong-password","new_password":"replacement-password"}`, 10006)
	change(token, `{"old_password":"original-password","new_password":"replacement-password"}`, 0)
	change(token, `{"old_password":"replacement-password","new_password":"stale-token-password"}`, 40100)
	var current sysmodel.SysUser
	requireBusinessError(t, s.db.WithContext(ctx).First(&current, u.ID).Error, nil)
	if current.TokenVersion != u.TokenVersion+1 || bcrypt.CompareHashAndPassword([]byte(current.PasswordHash), []byte("replacement-password")) != nil {
		t.Fatal("successful change did not atomically replace password and revoke token")
	}
	newToken, err := jwt.Generate(cfg.JWT.Secret, time.Hour, u.ID, u.Username, current.TokenVersion, 11)
	requireBusinessError(t, err, nil)
	change(newToken, `{"old_password":"replacement-password","new_password":"fresh-login-password"}`, 0)
}
