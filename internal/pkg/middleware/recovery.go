package middleware

import (
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/response"
)

// Recovery panic 兜底，防止进程退出。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.WithContext(c.Request.Context()).Error("gin panic recovered",
					"err", r, "path", c.Request.URL.Path, "stack", string(debug.Stack()))
				response.Fail(c, errcode.Internal)
				c.Abort()
			}
		}()
		c.Next()
	}
}
