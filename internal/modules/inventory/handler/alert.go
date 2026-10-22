package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

func (h *Handler) listAlerts(c *gin.Context) {
	var q dto.AlertQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	result, err := h.svc.ListAlerts(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) saveAlertRule(c *gin.Context) {
	var req dto.SaveAlertRuleReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	result, err := h.svc.SaveAlertRule(c.Request.Context(), &req, middleware.Username(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) deleteAlertRule(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteAlertRule(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) alertOptions(c *gin.Context) {
	var q dto.AlertOptionQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	rows, total, err := h.svc.AlertOptions(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, rows, total)
}
