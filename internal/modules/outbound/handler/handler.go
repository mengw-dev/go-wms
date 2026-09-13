package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	g := auth.Group("/outbound")
	perm := func(action string) gin.HandlerFunc {
		return middleware.Permission(checker, "wms:outbound:"+action)
	}

	read := perm("view")
	orders := g.Group("/orders")
	{
		orders.GET("", read, h.list)
		orders.GET("/:id", read, h.get)
		orders.POST("", perm("create"), h.create)
		orders.DELETE("/:id", perm("create"), h.delete)
		orders.POST("/:id/submit", perm("submit"), h.submit)
		orders.POST("/:id/approve", perm("approve"), h.approve)
		orders.POST("/:id/cancel", perm("cancel"), h.cancel)
	}

	g.POST("/tasks/:id/pick", perm("pick"), h.pick)
}

func (h *Handler) list(c *gin.Context) {
	var q dto.OrderQuery
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

func (h *Handler) get(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	detail, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

func (h *Handler) create(c *gin.Context) {
	var req dto.CreateOrderReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	order, err := h.svc.Create(c.Request.Context(), &req, middleware.Username(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, order)
}

func (h *Handler) delete(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) submit(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Submit(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) approve(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Approve(c.Request.Context(), id, middleware.Username(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) cancel(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id, middleware.Username(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) pick(c *gin.Context) {
	var req dto.PickReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Pick(c.Request.Context(), req.TaskID, req.Qty, middleware.Username(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}
