package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/response"
)

func (h *Handler) scanOrderSKU(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var q dto.ScanQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	rows, err := h.svc.ScanOrderSKU(c.Request.Context(), id, q.Code)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rows)
}
