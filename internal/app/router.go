package app

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

// NewRouter 构建路由与中间件链：
// RequestID → CORS → Recovery → AccessLog → Auth(JWT) → OperLog(异步审计) → Permission(按路由)。
func (a *App) NewRouter() *gin.Engine {
	if a.Config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.CORS(), middleware.Recovery(), middleware.AccessLog())

	// 健康检查：DB 不可用必须返回非 2xx，K8s 探针/负载均衡才能摘除故障实例
	r.GET("/healthz", func(c *gin.Context) {
		if err := a.healthz(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "down", "error": err.Error()})
			return
		}
		response.OK(c, gin.H{"status": "ok"})
	})

	api := r.Group("/api/v1")
	// 登录路由：仅鉴权链路之外
	pub := api.Group("")
	// 需登录的路由：JWT → 操作日志审计
	auth := api.Group("", middleware.Auth(a.Config.JWT.Secret, a.SystemAPI), middleware.OperLog(a.SystemAPI))

	a.SysHandler.RegisterRoutes(pub, auth, a.SystemAPI)
	a.BasicHandler.RegisterRoutes(auth, a.SystemAPI)
	a.InvHandler.RegisterRoutes(auth, a.SystemAPI)
	a.TaskHandler.RegisterRoutes(auth, a.SystemAPI)
	a.InboundHandler.RegisterRoutes(auth, a.SystemAPI)
	a.OutboundHandler.RegisterRoutes(auth, a.SystemAPI)
	a.StocktakeHandler.RegisterRoutes(auth, a.SystemAPI)

	return r
}

// healthz 健康检查：DB 必须可用，Redis 不可用不影响健康（已降级运行）。
func (a *App) healthz() error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
