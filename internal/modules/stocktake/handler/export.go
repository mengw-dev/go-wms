package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

func (h *Handler) exportDetails(c *gin.Context) {
	var q dto.ExportQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	file, err := h.svc.ExportDetails(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Excel(c, file.Name, file.Data)
}
