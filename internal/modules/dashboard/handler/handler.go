// Package handler exposes permission-filtered dashboard queries.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/dashboard/dto"
	"gowms/internal/modules/dashboard/service"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

type Handler struct{ svc *service.Service }

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes relies on the authenticated group; each section checks its existing business permission.
func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	auth.GET("/dashboard/overview", func(c *gin.Context) { h.overview(c, checker) })
}

func (h *Handler) overview(c *gin.Context, checker middleware.PermsChecker) {
	id, ok := httpx.OptionalQueryID(c, "warehouse_id")
	if !ok {
		return
	}
	has := func(perm string) bool { return checker.HasPerm(c.Request.Context(), middleware.UserID(c), perm) }
	access := dto.Access{Inbound: has("wms:inbound:view"), Outbound: has("wms:outbound:view"), Tasks: has("wms:task"),
		Inventory: has("wms:inventory"), Basic: has("wms:basic"), Stocktake: has("wms:stocktake:view")}
	out, err := h.svc.Overview(c.Request.Context(), id, access, time.Now())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, out)
}
