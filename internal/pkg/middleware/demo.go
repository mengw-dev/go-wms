package middleware

import (
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

// DemoSessionValidator 由 demo 模块实现，验证当前用户是否持有有效演示会话。
type DemoSessionValidator interface {
	Enabled() bool
	// IsDemoTenant 判断已认证请求是否属于配置的演示租户。
	IsDemoTenant(ctx context.Context) bool
	ValidateSession(ctx context.Context, sessionID string) error
}

// DemoSession 限制每个演示账号同时只能有一个有效会话（会话锁按租户隔离）。
func DemoSession(validator DemoSessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if validator == nil || !validator.Enabled() || !validator.IsDemoTenant(c.Request.Context()) {
			c.Next()
			return
		}
		// 登录后的首个请求是获取会话，不能在此时要求已经持有会话。
		if c.Request.Method == "POST" && c.Request.URL.Path == "/api/v1/demo/session/acquire" {
			c.Next()
			return
		}
		sessionID := strings.TrimSpace(c.GetHeader("X-Demo-Session"))
		if sessionID == "" {
			response.Fail(c, errcode.DemoSessionInvalid)
			c.Abort()
			return
		}
		if err := validator.ValidateSession(c.Request.Context(), sessionID); err != nil {
			response.Fail(c, err)
			c.Abort()
			return
		}
		c.Next()
	}
}
