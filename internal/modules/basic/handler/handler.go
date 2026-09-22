package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/service"
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
	g := auth.Group("/basic")
	write := middleware.Permission(checker, "wms:basic")
	read := middleware.Permission(checker, "wms:basic")

	wh := g.Group("/warehouses")
	{
		wh.GET("", read, h.listWarehouses)
		wh.POST("", write, h.createWarehouse)
		wh.PUT("/:id", write, h.updateWarehouse)
		wh.DELETE("/:id", write, h.deleteWarehouse)
		wh.PUT("/:id/status", write, h.warehouseStatus)
	}

	loc := g.Group("/locations")
	{
		loc.GET("", read, h.listLocations)
		loc.POST("/batch", write, h.batchCreateLocations)
		loc.PUT("/:id/status", write, h.locationStatus)
		loc.DELETE("/:id", write, h.deleteLocation)
	}

	sku := g.Group("/skus")
	{
		sku.GET("", read, h.listSKUs)
		sku.GET("/barcode/:barcode", read, h.getByBarcode)
		sku.POST("", write, h.createSKU)
		sku.PUT("/:id", write, h.updateSKU)
		sku.DELETE("/:id", write, h.deleteSKU)
	}
}

// ---------- 仓库 ----------

func (h *Handler) listWarehouses(c *gin.Context) {
	var q dto.WarehouseQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListWarehouseResponses(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) createWarehouse(c *gin.Context) {
	var req dto.WarehouseReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.CreateWarehouse(c.Request.Context(), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) updateWarehouse(c *gin.Context) {
	var req dto.WarehouseReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.UpdateWarehouse(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) warehouseStatus(c *gin.Context) {
	var req dto.StatusReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.UpdateWarehouseStatus(c.Request.Context(), id, *req.Status); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) deleteWarehouse(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteWarehouse(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------- 库位 ----------

func (h *Handler) listLocations(c *gin.Context) {
	var q dto.LocationQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListLocationResponses(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) batchCreateLocations(c *gin.Context) {
	var req dto.LocationBatchReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	n, err := h.svc.BatchCreateLocations(c.Request.Context(), &req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, dto.LocationBatchResp{Created: n})
}

func (h *Handler) locationStatus(c *gin.Context) {
	var req dto.StatusReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.UpdateLocationStatus(c.Request.Context(), id, *req.Status); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) deleteLocation(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteLocation(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

// ---------- SKU ----------

func (h *Handler) listSKUs(c *gin.Context) {
	var q dto.CommonQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListSKUResponses(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) getByBarcode(c *gin.Context) {
	sku, err := h.svc.GetSKUByBarcodeResponse(c.Request.Context(), c.Param("barcode"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, sku)
}

func (h *Handler) createSKU(c *gin.Context) {
	var req dto.SKUReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.CreateSKU(c.Request.Context(), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) updateSKU(c *gin.Context) {
	var req dto.SKUReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.UpdateSKU(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) deleteSKU(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteSKU(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}
