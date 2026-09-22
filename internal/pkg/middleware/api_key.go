package middleware

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
	"gowms/internal/pkg/tenant"
)

// APIKey 校验外部系统集成请求的 X-API-Key。
func APIKey(expected string, tenantID int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(expected) == "" || tenantID < 0 {
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
		// 租户由服务器配置绑定到凭据，不接受客户端头部或请求体指定的租户。
		c.Request = c.Request.WithContext(tenant.WithExactTenant(c.Request.Context(), tenantID))
		c.Next()
	}
}
