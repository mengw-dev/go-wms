package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

func (h *Handler) registerClosure(g *gin.RouterGroup, checker middleware.PermsChecker) {
	read := middleware.Permission(checker, "wms:outbound:view")
	exceptions := middleware.Permission(checker, "wms:outbound:exception")
	returns := middleware.Permission(checker, "wms:outbound:return")
	g.GET("/exceptions", read, h.exceptions)
	g.POST("/exceptions", read, exceptions, h.reportException)
	g.POST("/exceptions/:id/resolve", read, exceptions, h.resolveException)
	g.POST("/waves/:id/abort", read, exceptions, h.abortWave)
	g.GET("/returns", read, h.returns)
	g.GET("/orders/:id/returnable", read, h.returnable)
	g.GET("/orders/:id/return-locations", read, returns, h.returnLocations)
	g.POST("/returns", read, returns, h.receiveReturn)
}
func (h *Handler) returnLocations(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var q dto.ReturnLocationQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	rows, n, e := h.svc.ReturnLocations(c.Request.Context(), id, &q)
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OKPage(c, rows, n)
}
func (h *Handler) returnable(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	rows, e := h.svc.Returnable(c.Request.Context(), id)
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OK(c, rows)
}
func (h *Handler) exceptions(c *gin.Context) {
	var q dto.ClosureQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	rows, n, e := h.svc.ListExceptions(c.Request.Context(), &q)
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OKPage(c, rows, n)
}
func (h *Handler) returns(c *gin.Context) {
	var q dto.ClosureQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	rows, n, e := h.svc.ListReturns(c.Request.Context(), &q)
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OKPage(c, rows, n)
}
func (h *Handler) reportException(c *gin.Context) {
	if !httpx.CommandKey(c) {
		return
	}
	var req dto.ExceptionRequest
	if !httpx.BindStrictJSON(c, &req) {
		return
	}
	out, e := h.svc.ReportException(c.Request.Context(), &req, middleware.Username(c), c.GetHeader("Idempotency-Key"))
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OK(c, out)
}
func (h *Handler) resolveException(c *gin.Context) {
	if !httpx.CommandKey(c) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req dto.ResolveExceptionRequest
	if !httpx.BindStrictJSON(c, &req) {
		return
	}
	out, e := h.svc.ResolveWorkException(c.Request.Context(), id, &req, middleware.Username(c), c.GetHeader("Idempotency-Key"))
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OK(c, out)
}
func (h *Handler) receiveReturn(c *gin.Context) {
	if !httpx.CommandKey(c) {
		return
	}
	var req dto.CustomerReturnRequest
	if !httpx.BindStrictJSON(c, &req) {
		return
	}
	out, e := h.svc.ReceiveCustomerReturn(c.Request.Context(), &req, middleware.Username(c), c.GetHeader("Idempotency-Key"))
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OK(c, out)
}
func (h *Handler) abortWave(c *gin.Context) {
	if !httpx.CommandKey(c) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var req dto.AbortWaveRequest
	if !httpx.BindStrictJSON(c, &req) {
		return
	}
	out, e := h.svc.AbortWorkWave(c.Request.Context(), id, &req, middleware.Username(c), c.GetHeader("Idempotency-Key"))
	if e != nil {
		response.Fail(c, e)
		return
	}
	response.OK(c, out)
}
