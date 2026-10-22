package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

func (h *Handler) exportInventory(c *gin.Context) {
	var q dto.InventoryExportQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	file, err := h.svc.ExportInventory(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Excel(c, file.Name, file.Data)
}

func (h *Handler) exportTransactions(c *gin.Context) {
	var q dto.TransExportQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	file, err := h.svc.ExportTransactions(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Excel(c, file.Name, file.Data)
}
