package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

// RegisterIntegrationRoutes 注册无 JWT、使用 API Key 的外部系统对接路由。
func (h *Handler) RegisterIntegrationRoutes(pub *gin.RouterGroup, apiKey string, tenantID int64) {
	g := pub.Group("/integration")
	g.POST("/outbound-orders", middleware.APIKey(apiKey, tenantID), h.createExternalOrder)
}

func (h *Handler) createExternalOrder(c *gin.Context) {
	var req dto.ExternalCreateOrderReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	order, idempotent, err := h.svc.CreateExternal(c.Request.Context(), &req, "integration")
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, dto.ExternalCreateOrderResp{
		OrderID: order.ID, OrderNo: order.OrderNo, BizOrderNo: order.BizOrderNo,
		Status: string(order.Status), Idempotent: idempotent,
	})
}
