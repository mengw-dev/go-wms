package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/task/service"
	"gowms/internal/pkg/errcode"
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
	orderID, _ := strconv.ParseInt(c.Query("order_id"), 10, 64)
	taskType := c.Query("task_type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 10
	}
	list, total, err := h.svc.List(c.Request.Context(), orderID, taskType, page, size)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	t, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, errcode.TaskNotFound)
		return
	}
	response.OK(c, t)
}
