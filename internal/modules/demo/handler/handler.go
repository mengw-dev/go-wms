package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/demo/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

// Handler 暴露演示会话、场景执行和数据重置接口。
type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	g := auth.Group("/demo")
	perm := middleware.Permission(checker, "wms:demo")

	g.POST("/session/acquire", perm, h.acquire)
	g.POST("/session/heartbeat", perm, h.heartbeat)
	g.POST("/session/release", perm, h.release)
	g.GET("/session/status", perm, h.status)
	g.POST("/run/inbound", perm, h.runInbound)
	g.POST("/run/outbound", perm, h.runOutbound)
	g.POST("/run/stocktake", perm, h.runStocktake)
	g.POST("/run/full", perm, h.runFull)
	g.POST("/run/inbound_drafts", perm, h.runInboundDrafts)
	g.POST("/run/outbound_drafts", perm, h.runOutboundDrafts)
	g.POST("/run/stocktake_drafts", perm, h.runStocktakeDrafts)
	g.POST("/run/concurrent", perm, h.runConcurrent)
	g.GET("/performance", perm, h.performance)
	g.GET("/activity", perm, h.activity)
	g.POST("/reset", perm, h.reset)
}

func (h *Handler) acquire(c *gin.Context) {
	info, err := h.svc.AcquireSession(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, info)
}

func (h *Handler) heartbeat(c *gin.Context) {
	info, err := h.svc.Heartbeat(c.Request.Context(), demoSessionID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, info)
}

func (h *Handler) release(c *gin.Context) {
	if err := h.svc.ReleaseSession(c.Request.Context(), demoSessionID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) status(c *gin.Context) {
	info, ok := h.svc.SessionStatus(c.Request.Context(), demoSessionID(c))
	if !ok {
		response.Fail(c, errcode.DemoSessionInvalid)
		return
	}
	response.OK(c, info)
}

func (h *Handler) runInbound(c *gin.Context)   { h.run(c, service.ScenarioInbound) }
func (h *Handler) runOutbound(c *gin.Context)  { h.run(c, service.ScenarioOutbound) }
func (h *Handler) runStocktake(c *gin.Context) { h.run(c, service.ScenarioStocktake) }
func (h *Handler) runFull(c *gin.Context)      { h.run(c, service.ScenarioFull) }
func (h *Handler) runInboundDrafts(c *gin.Context) {
	h.run(c, service.ScenarioInboundDrafts)
}
func (h *Handler) runOutboundDrafts(c *gin.Context) {
	h.run(c, service.ScenarioOutboundDrafts)
}
func (h *Handler) runStocktakeDrafts(c *gin.Context) {
	h.run(c, service.ScenarioStocktakeDrafts)
}

func (h *Handler) runConcurrent(c *gin.Context) {
	var req struct {
		Concurrency int `json:"concurrency" binding:"omitempty,min=1,max=30"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && c.Request.ContentLength > 0 {
		response.Fail(c, errcode.ParamError)
		return
	}
	result, err := h.svc.RunConcurrent(c.Request.Context(), demoSessionID(c), req.Concurrency)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) performance(c *gin.Context) {
	result, err := h.svc.Performance(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) activity(c *gin.Context) {
	limit, ok := httpx.QueryInt(c, "limit", 20, 1, 50)
	if !ok {
		return
	}
	result, err := h.svc.Activity(c.Request.Context(), limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) run(c *gin.Context, scenario string) {
	result, err := h.svc.Run(c.Request.Context(), demoSessionID(c), scenario)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) reset(c *gin.Context) {
	if err := h.svc.Reset(c.Request.Context(), demoSessionID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func demoSessionID(c *gin.Context) string {
	return c.GetHeader("X-Demo-Session")
}
