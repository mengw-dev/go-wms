package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/service"
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
	g := auth.Group("/outbound")
	perm := func(action string) gin.HandlerFunc {
		return middleware.Permission(checker, "wms:outbound:"+action)
	}

	read := perm("view")
	orders := g.Group("/orders")
	{
		orders.GET("", read, h.list)
		orders.GET("/:id", read, h.get)
		orders.POST("", perm("create"), h.create)
		orders.DELETE("/:id", perm("create"), h.delete)
		orders.POST("/batch-delete", perm("create"), h.batchDelete)
		orders.POST("/batch-submit", perm("submit"), h.batchSubmit)
		orders.POST("/batch-approve", perm("approve"), h.batchApprove)
		orders.POST("/batch-cancel", perm("cancel"), h.batchCancel)
		orders.POST("/:id/submit", perm("submit"), h.submit)
		orders.POST("/:id/approve", perm("approve"), h.approve)
		orders.POST("/:id/cancel", perm("cancel"), h.cancel)
	}

	g.POST("/tasks/:id/pick", perm("pick"), h.pick)
}

// RegisterPDARoutes 挂载 PDA 专用路由：现场作业走强制扫码入口
// （库位必填，任务有批次时批次必填），领取与拣货都强制携带 Idempotency-Key，
// 后台接口保持宽松语义。
func (h *Handler) RegisterPDARoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	perm := middleware.Permission(checker, "wms:outbound:pick")
	auth.POST("/pda/tasks/:id/claim", perm, h.pdaClaim)
	auth.POST("/pda/tasks/:id/pick", perm, h.pdaPick)
}

func (h *Handler) list(c *gin.Context) {
	var q dto.OrderQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListResponses(c.Request.Context(), &q)
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
	detail, err := h.svc.GetResponse(c.Request.Context(), id)
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
	order, err := h.svc.CreateResponse(c.Request.Context(), &req, middleware.Username(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, order)
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
	if err := h.svc.Cancel(c.Request.Context(), id, middleware.Username(c)); err != nil {
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
	response.OK(c, h.svc.BatchCancel(c.Request.Context(), req.IDs, middleware.Username(c)))
}

func (h *Handler) pick(c *gin.Context) { h.pickTask(c, false) }

// pdaPick PDA 入口：强制携带 Idempotency-Key 并扫描库位（任务有批次时批次必填），服务端再次校验。
func (h *Handler) pdaPick(c *gin.Context) { h.pickTask(c, true) }

// pdaClaim 领取（续领）拣货任务：强制携带 Idempotency-Key（重试回放同一凭证），返回领取凭证与任务快照。
func (h *Handler) pdaClaim(c *gin.Context) {
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if strings.TrimSpace(idempotencyKey) == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	taskID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	result, err := h.svc.ClaimPickTask(c.Request.Context(), taskID, middleware.Username(c), idempotencyKey)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}

// pickTask 两个拣货入口共用的处理：绑定 → 调用服务 → 成功返回任务快照，
// 业务拒绝时随错误带回最新快照（PDA 就地刷新，不必退出重进）。
func (h *Handler) pickTask(c *gin.Context, strict bool) {
	// PDA 入口强制请求级幂等：现场重放必须携带 Idempotency-Key，缺失直接拒绝；
	// 后台入口保持可选（兼容脚本/联调调用）。
	idempotencyKey := c.GetHeader("Idempotency-Key")
	if strict && strings.TrimSpace(idempotencyKey) == "" {
		response.Fail(c, errcode.ParamError)
		return
	}
	var req dto.PickReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	taskID, ok := httpx.PathID(c)
	if !ok {
		return
	}
	scan := &service.PickScan{LocationCode: req.LocationCode, BatchNo: req.BatchNo, Strict: strict}
	result, err := h.svc.Pick(c.Request.Context(), taskID, req.Qty, middleware.Username(c), scan, req.ClaimToken, idempotencyKey)
	if err != nil {
		if result != nil {
			response.FailWithData(c, err, result)
			return
		}
		response.Fail(c, err)
		return
	}
	response.OK(c, result)
}
