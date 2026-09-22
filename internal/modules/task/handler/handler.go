package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/task/service"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes 任务查询路由（业务动作在各业务模块）。
func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	read := middleware.Permission(checker, "wms:task")
	auth.GET("/tasks", read, h.list)
	auth.GET("/tasks/:id", read, h.get)
}

func (h *Handler) list(c *gin.Context) {
	orderID, ok := httpx.OptionalQueryID(c, "order_id")
	if !ok {
		return
	}
	taskType := c.Query("task_type")
	status := c.Query("status")
	keyword := c.Query("keyword")
	page, ok := httpx.QueryInt(c, "page", 1, 1, 100000)
	if !ok {
		return
	}
	size, ok := httpx.QueryInt(c, "page_size", 10, 1, 100)
	if !ok {
		return
	}
	list, total, err := h.svc.ListResponses(c.Request.Context(), orderID, taskType, status, keyword, page, size)
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
	t, err := h.svc.GetResponse(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, t)
}
