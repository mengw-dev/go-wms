package handler

import (
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

// RegisterIntegrationRoutes 注册无 JWT、使用 API Key 的外部系统对接路由。
func (h *Handler) RegisterIntegrationRoutes(pub *gin.RouterGroup, apiKey string, tenantID int64) {
	g := pub.Group("/integration")
	g.Use(middleware.APIKey(apiKey, tenantID))
	g.POST("/outbound-orders", h.createExternalOrder)
	g.GET("/outbound-orders/by-biz-no/:bizNo", h.getExternalOrder)
	g.POST("/outbound-orders/by-biz-no/:bizNo/cancel", h.cancelExternalOrder)
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

func validBizNo(c *gin.Context) (string, bool) {
	value := strings.TrimSpace(c.Param("bizNo"))
	if value == "" || utf8.RuneCountInString(value) > 64 {
		response.Fail(c, errcode.ParamError)
		return "", false
	}
	return value, true
}

func externalResponse(order *model.ShipmentOrder) dto.ExternalOrderResp {
	return dto.ExternalOrderResp{OrderID: order.ID, OrderNo: order.OrderNo, BizOrderNo: order.BizOrderNo,
		Status: string(order.Status), ExpectedQty: order.ExpectedQty, AllocatedQty: order.AllocatedQty,
		PickedQty: order.PickedQty, UpdatedAt: order.UpdatedAt}
}

func (h *Handler) getExternalOrder(c *gin.Context) {
	bizNo, ok := validBizNo(c)
	if !ok {
		return
	}
	order, err := h.svc.GetExternal(c.Request.Context(), bizNo)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, externalResponse(order))
}

func (h *Handler) cancelExternalOrder(c *gin.Context) {
	bizNo, ok := validBizNo(c)
	if !ok {
		return
	}
	order, err := h.svc.CancelExternal(c.Request.Context(), bizNo)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, externalResponse(order))
}
