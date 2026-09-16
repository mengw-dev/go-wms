package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

// APIKey 校验外部系统集成请求的 X-API-Key。
func APIKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(expected) == "" {
			response.Fail(c, errcode.IntegrationDisabled)
			c.Abort()
			return
		}
		provided := strings.TrimSpace(c.GetHeader("X-API-Key"))
		if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			response.Fail(c, errcode.IntegrationUnauthorized)
			c.Abort()
			return
		}
		c.Next()
	}
}
