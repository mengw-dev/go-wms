package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes 注册库存查询、预警与仓内移库；出入库扣增仍由业务模块调用 API 完成。
func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	g := auth.Group("/inventory")
	read := middleware.Permission(checker, "wms:inventory")
	move := middleware.Permission(checker, "wms:inventory:move")
	g.POST("/freeze", read, middleware.Permission(checker, "wms:inventory:freeze"), h.freeze)
	g.GET("/move-source/:id", read, move, h.moveSource)
	g.GET("/moves", read, h.movements)
	g.GET("/move-locations", read, move, h.moveLocations)
	g.POST("/moves", read, move, h.moveStock)
	manageAlerts := middleware.Permission(checker, "wms:inventory:alert:manage")
	g.GET("/export", read, h.exportInventory)
	g.GET("/trans/export", read, h.exportTransactions)
	g.GET("/alerts", read, h.listAlerts)
	g.GET("/alert-options", manageAlerts, h.alertOptions)
	g.PUT("/alert-rules", manageAlerts, h.saveAlertRule)
	g.DELETE("/alert-rules/:id", manageAlerts, h.deleteAlertRule)
	{
		g.GET("", read, h.list)
		g.GET("/summary", read, h.summary)
		g.GET("/trans", read, h.listTrans)
	}
}

func (h *Handler) list(c *gin.Context) {
	var q dto.InventoryQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.List(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) summary(c *gin.Context) {
	var q dto.SummaryQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.SummaryBySKU(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) listTrans(c *gin.Context) {
	var q dto.TransQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListTrans(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}
