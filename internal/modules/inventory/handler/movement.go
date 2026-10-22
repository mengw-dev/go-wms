package handler

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

func (h *Handler) moveSource(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	result, err := h.svc.MoveSource(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

func (h *Handler) moveStock(c *gin.Context) {
	var req dto.MoveRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		if httpx.IsBodyTooLarge(err) {
			response.Fail(c, errcode.PayloadTooLarge)
		} else {
			response.Fail(c, errcode.ParamError)
		}
		return
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if httpx.IsBodyTooLarge(err) {
			response.Fail(c, errcode.PayloadTooLarge)
		} else {
			response.Fail(c, errcode.ParamError)
		}
		return
	}
	if binding.Validator.ValidateStruct(&req) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	result, err := h.svc.MoveStock(c.Request.Context(), &req, c.GetHeader("Idempotency-Key"), middleware.UserID(c), middleware.Username(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}
func (h *Handler) movements(c *gin.Context) {
	var q dto.MoveQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListMovements(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}
func (h *Handler) moveLocations(c *gin.Context) {
	var q dto.MoveLocationQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.MoveLocations(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}
