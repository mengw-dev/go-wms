package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/log"
)

// context key 与取值辅助。
type ctxKey string

const (
	ctxRequestID ctxKey = "request_id"
	ctxUserID    ctxKey = "user_id"
	ctxUsername  ctxKey = "username"
	ctxTenantID  ctxKey = "tenant_id"
)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := normalizeRequestID(c.GetHeader("X-Request-ID"))
		if id == "" {
			id = newRequestID()
		}
		c.Set(string(ctxRequestID), id)
		c.Writer.Header().Set("X-Request-ID", id)
		c.Request = c.Request.WithContext(log.WithRequestID(c.Request.Context(), id))
		c.Next()
	}
}

// UserID returns the authenticated user ID stored by Auth.
func UserID(c *gin.Context) int64 {
	value, _ := c.Get(string(ctxUserID))
	userID, _ := value.(int64)
	return userID
}

// Username returns the authenticated username stored by Auth.
func Username(c *gin.Context) string {
	value, _ := c.Get(string(ctxUsername))
	username, _ := value.(string)
	return username
}

// RequestIDOf returns the request ID injected by RequestID.
func RequestIDOf(c *gin.Context) string {
	value, _ := c.Get(string(ctxRequestID))
	requestID, _ := value.(string)
	return requestID
}

// TenantIDOf 取当前请求的租户 ID（来自 JWT claims；0 表示平台/默认租户）。
func TenantIDOf(c *gin.Context) int64 {
	value, _ := c.Get(string(ctxTenantID))
	tenantID, _ := value.(int64)
	return tenantID
}

func normalizeRequestID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" || len(id) > 128 {
		return ""
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':' {
			continue
		}
		return ""
	}
	return id
}

func newRequestID() string {
	return time.Now().Format("20060102150405") + randHex()
}
