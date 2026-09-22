package middleware

import (
	"time"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/log"
)

// AccessLog 访问日志：方法/路径/状态/耗时/操作人，>500ms 打 warn。
func AccessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		cost := time.Since(start).Milliseconds()
		attrs := []any{
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"cost_ms", cost,
			"ip", c.ClientIP(),
			"request_id", RequestIDOf(c),
			"user_id", UserID(c),
		}
		l := log.WithContext(c.Request.Context())
		if cost > 500 {
			l.Warn("slow request", attrs...)
		} else {
			l.Info("access", attrs...)
		}
	}
}
