package handler

import (
	"io"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/service"
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
	g := auth.Group("/inbound")
	perm := func(action string) gin.HandlerFunc {
		return middleware.Permission(checker, "wms:inbound:"+action)
	}

	read := perm("view")
	orders := g.Group("/orders")
	{
		orders.GET("", read, h.list)
		orders.GET("/:id", read, h.get)
		orders.POST("", perm("create"), h.create)
		orders.PUT("/:id", perm("create"), h.update)
		orders.DELETE("/:id", perm("create"), h.delete)
		orders.POST("/batch-delete", perm("create"), h.batchDelete)
		orders.POST("/batch-submit", perm("submit"), h.batchSubmit)
		orders.POST("/batch-approve", perm("approve"), h.batchApprove)
		orders.POST("/batch-cancel", perm("cancel"), h.batchCancel)
		orders.POST("/:id/submit", perm("submit"), h.submit)
		orders.POST("/:id/approve", perm("approve"), h.approve)
		orders.POST("/:id/cancel", perm("cancel"), h.cancel)
		orders.POST("/:id/receive", perm("receive"), h.receive)
	}

	g.POST("/tasks/:id/putaway", perm("putaway"), h.putaway)
	g.POST("/import", perm("create"), h.importExcel)
	g.GET("/import/:taskId", read, h.importStatus)
	g.DELETE("/import/:taskId/orders", perm("create"), h.deleteByImportTask)
}

func (h *Handler) list(c *gin.Context) {
	var q dto.OrderQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.List(c.Request.Context(), &q)
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
	detail, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

func (h *Handler) create(c *gin.Context) {
	var req dto.CreateOrderReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	order, err := h.svc.Create(c.Request.Context(), &req, middleware.Username(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, order)
}

func (h *Handler) update(c *gin.Context) {
	var req dto.CreateOrderReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Update(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) delete(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) submit(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Submit(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) approve(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Approve(c.Request.Context(), id, middleware.Username(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) cancel(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	if err := h.svc.Cancel(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) batchDelete(c *gin.Context) {
	var req dto.BatchOperReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	response.OK(c, h.svc.BatchDelete(c.Request.Context(), req.IDs))
}

func (h *Handler) batchSubmit(c *gin.Context) {
	var req dto.BatchOperReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	response.OK(c, h.svc.BatchSubmit(c.Request.Context(), req.IDs))
}

func (h *Handler) batchApprove(c *gin.Context) {
	var req dto.BatchOperReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	response.OK(c, h.svc.BatchApprove(c.Request.Context(), req.IDs, middleware.Username(c)))
}

func (h *Handler) batchCancel(c *gin.Context) {
	var req dto.BatchOperReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	response.OK(c, h.svc.BatchCancel(c.Request.Context(), req.IDs))
}

func (h *Handler) deleteByImportTask(c *gin.Context) {
	taskID := c.Param("taskId")
	resp, err := h.svc.DeleteByImportTask(c.Request.Context(), taskID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

func (h *Handler) receive(c *gin.Context) {
	var req dto.ReceiveReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	detailID, ok := httpx.OptionalQueryID(c, "detail_id")
	if !ok {
		return
	}
	if detailID == 0 {
		detailID = req.DetailID
	}
	if err := h.svc.Receive(c.Request.Context(), id, detailID, &req, middleware.Username(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) putaway(c *gin.Context) {
	var req dto.PutawayReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	if err := h.svc.Putaway(c.Request.Context(), req.TaskID, req.LocationID, req.Qty, middleware.Username(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

const maxImportFileSize = 10 << 20

func (h *Handler) importExcel(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		if httpx.IsBodyTooLarge(err) {
			response.Fail(c, errcode.PayloadTooLarge)
			return
		}
		response.Fail(c, errcode.ImportFileInvalid)
		return
	}
	if file.Size <= 0 || file.Size > maxImportFileSize {
		response.Fail(c, errcode.PayloadTooLarge)
		return
	}
	if strings.ToLower(filepath.Ext(file.Filename)) != ".xlsx" {
		response.Fail(c, errcode.ImportFileInvalid)
		return
	}
	f, err := file.Open()
	if err != nil {
		response.Fail(c, errcode.ImportFileInvalid)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		if httpx.IsBodyTooLarge(err) {
			response.Fail(c, errcode.PayloadTooLarge)
			return
		}
		response.Fail(c, errcode.ImportFileInvalid)
		return
	}
	if len(data) == 0 {
		response.Fail(c, errcode.ImportFileInvalid)
		return
	}

	resp, err := h.svc.Import(c.Request.Context(), file.Filename, data)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

func (h *Handler) importStatus(c *gin.Context) {
	task, err := h.svc.GetImport(c.Request.Context(), c.Param("taskId"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, task)
}
