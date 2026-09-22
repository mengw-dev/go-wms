package middleware

import (
	"context"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/response"
	"gowms/internal/pkg/tenant"
)

// AuthValidator 在签名校验后复核用户状态和 Token 版本，使禁用用户/改密后的旧 Token 立即失效。
type AuthValidator interface {
	ValidateToken(ctx context.Context, userID int64, tokenVersion int) error
}

// Auth JWT 鉴权：解析 Bearer Token，复核用户状态，注入 userID/username。
func Auth(secret string, validator AuthValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		} else {
			response.Fail(c, errcode.Unauthorized)
			c.Abort()
			return
		}
		claims, err := jwt.Parse(secret, token)
		if err != nil {
			response.Fail(c, errcode.Unauthorized)
			c.Abort()
			return
		}
		// 签名有效后，数据库复核也必须限定在 Token 声明的租户内。
		c.Request = c.Request.WithContext(tenant.WithTenant(c.Request.Context(), claims.TenantID))
		if validator != nil {
			if err := validator.ValidateToken(c.Request.Context(), claims.UserID, claims.TokenVersion); err != nil {
				response.Fail(c, err)
				c.Abort()
				return
			}
		}
		c.Set(string(ctxUserID), claims.UserID)
		c.Set(string(ctxUsername), claims.Username)
		c.Set(string(ctxTenantID), claims.TenantID)
		// ctx 链式叠加：保留 RequestID，再挂 UserID（日志）与租户 ID（GORM 隔离回调）
		c.Request = c.Request.WithContext(
			tenant.WithTenant(log.WithUserID(c.Request.Context(), claims.UserID), claims.TenantID))
		c.Next()
	}
}

// PermsChecker 由 system 模块实现，供 Permission 中间件校验权限。
type PermsChecker interface {
	HasPerm(ctx context.Context, userID int64, perm string) bool
}

// Permission 权限校验中间件。
func Permission(checker PermsChecker, perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !checker.HasPerm(c.Request.Context(), UserID(c), perm) {
			response.Fail(c, errcode.Forbidden)
			c.Abort()
			return
		}
		c.Next()
	}
}
