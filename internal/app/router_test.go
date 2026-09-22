package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"gowms/internal/pkg/config"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/middleware"
)

type routerSystemStub struct{}

func (routerSystemStub) ValidateToken(context.Context, int64, int) error  { return nil }
func (routerSystemStub) HasPerm(context.Context, int64, string) bool      { return false }
func (routerSystemStub) Record(context.Context, middleware.OperLogRecord) {}

// 必须通过实际 App 注册的路由验证，单测 DemoSession 本身无法发现注册顺序错误。
func TestBusinessRoutesRequireDemoSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("r", 32)}, Demo: config.DemoConfig{Enabled: true, Instances: 1}}
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	t.Cleanup(func() { _ = rdb.Close() })
	app := New(cfg, nil, rdb, nil)
	app.SystemAPI = routerSystemStub{}
	router, err := app.NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		username string
		tenantID int64
		want     int
	}{
		{"demo account", "demo1", 10001, http.StatusLocked},
		{"same name in ordinary tenant", "demo1", 22, http.StatusForbidden},
		{"other user in demo tenant", "regular", 10001, http.StatusLocked},
		{"platform account", "admin", 0, http.StatusForbidden},
	} {
		t.Run(tt.name, func(t *testing.T) {
			token, err := jwt.Generate(cfg.JWT.Secret, time.Hour, 42, tt.username, 1, tt.tenantID)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/api/v1/basic/skus", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != tt.want {
				t.Fatalf("status=%d want=%d body=%s", recorder.Code, tt.want, recorder.Body.String())
			}
		})
	}
}
