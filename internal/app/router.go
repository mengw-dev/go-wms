package app

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
	"gowms/internal/pkg/version"
)

// NewRouter 构建路由与中间件链：
// RequestID → CORS → Recovery → AccessLog → Auth(JWT) → OperLog(异步审计) → Permission(按路由)。
func (a *App) NewRouter() (*gin.Engine, error) {
	if a.Config.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	if a.Metrics != nil {
		r.Use(a.Metrics.Middleware())
	}
	if err := r.SetTrustedProxies(a.Config.TrustedProxyCIDRs()); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	r.Use(
		middleware.RequestID(),
		middleware.BodyLimit(a.Config.Server.BodyLimitMB),
		middleware.CORS(a.Config.CORSOrigins()),
		middleware.Recovery(),
		middleware.AccessLog(),
	)

	// 版本与特性开关：根路径供运维/探针使用，/api/v1 供前端公开查询（免登录）。
	versionHandler := func(c *gin.Context) {
		response.OK(c, gin.H{
			"version":          version.Version,
			"commit":           version.Commit,
			"build_time":       version.BuildTime,
			"demo_enabled":     a.Config.Demo.Enabled,
			"personal_enabled": a.Config.Personal.Enabled,
		})
	}
	r.GET("/version", versionHandler)

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
	pub.GET("/version", versionHandler)
	// 需登录的路由：JWT → 操作日志审计
	auth := api.Group("", middleware.Auth(a.Config.JWT.Secret, a.SystemAPI), middleware.OperLog(a.SystemAPI))

	a.SysHandler.RegisterRoutes(pub, auth, a.SystemAPI)
	a.BasicHandler.RegisterRoutes(auth, a.SystemAPI)
	a.InvHandler.RegisterRoutes(auth, a.SystemAPI)
	a.TaskHandler.RegisterRoutes(auth, a.SystemAPI)
	a.InboundHandler.RegisterRoutes(auth, a.SystemAPI)
	a.OutboundHandler.RegisterRoutes(auth, a.SystemAPI)
	a.OutboundHandler.RegisterIntegrationRoutes(pub, a.Config.Integration.APIKey)
	a.StocktakeHandler.RegisterRoutes(auth, a.SystemAPI)
	a.AIHandler.RegisterRoutes(auth, a.SystemAPI)

	// 演示模块特性门控：demo.enabled=false 时不挂载任何 /demo 路由与
	// DemoSession 中间件（生产环境直接 404，演示代码零暴露）。
	if a.Config.Demo.Enabled {
		// 免登录：登录页"在线体验"领取空闲演示账号（多租户多账号自动分配）
		a.DemoHandler.RegisterPublicRoutes(pub)
		auth.Use(middleware.DemoSession(a.DemoService))
		a.DemoHandler.RegisterRoutes(auth, a.SystemAPI)
	}

	// 持久体验账号（user1..userN）：登录页"个人空间"，数据长期保留。
	// 与演示模块相互独立：关闭演示模式时个人账号仍可单独开放。
	a.DemoHandler.RegisterPersonalPublicRoutes(pub)

	return r, nil
}

// healthz 健康检查：DB 必须可用，Redis 不可用不影响健康（已降级运行）。
func (a *App) healthz() error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}
