package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/response"
)

func (h *Handler) putawayLocations(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	var q dto.PutawayLocationQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	rows, total, err := h.svc.PutawayLocations(c.Request.Context(), id, &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, rows, total)
}
