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

// RegisterRoutes 库存查询路由（变更操作由 inbound/outbound/stocktake 通过 API 完成）。
func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	g := auth.Group("/inventory")
	read := middleware.Permission(checker, "wms:inventory")
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
