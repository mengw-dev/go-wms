package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

func (h *Handler) freeze(c *gin.Context) {
	if !httpx.CommandKey(c) {
		return
	}
	var req dto.FreezeRequest
	if !httpx.BindStrictJSON(c, &req) {
		return
	}
	id, err := h.svc.FreezeStock(c.Request.Context(), &req, c.GetHeader("Idempotency-Key"), middleware.Username(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, struct {
		InventoryID int64 `json:"inventory_id,string"`
	}{id})
}
