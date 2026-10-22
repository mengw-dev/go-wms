package handler

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/httpx"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

func (h *Handler) registerWaves(g *gin.RouterGroup, checker middleware.PermsChecker) {
	read := middleware.Permission(checker, "wms:outbound:view")
	manage := middleware.Permission(checker, "wms:outbound:approve")
	pick := middleware.Permission(checker, "wms:outbound:pick")
	w := g.Group("/waves")
	w.GET("", read, h.listWaves)
	w.GET("/candidates", read, h.waveCandidates)
	w.GET("/:id", read, h.getWave)
	w.POST("", manage, h.createWave)
	w.POST("/:id/claim", pick, h.waveAction("claim"))
	w.POST("/:id/cancel", manage, h.waveAction("cancel"))
	w.POST("/:id/lines/:part/collect", pick, h.waveAction("collect"))
	w.POST("/:id/items/:part/sort", pick, h.waveAction("sort"))
	w.POST("/:id/orders/:part/ship", pick, h.waveAction("ship"))
}

func (h *Handler) listWaves(c *gin.Context) {
	var q dto.WaveQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListWaves(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) waveCandidates(c *gin.Context) {
	var q dto.WaveCandidateQuery
	if c.ShouldBindQuery(&q) != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, err := h.svc.WaveCandidates(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

func (h *Handler) getWave(c *gin.Context) {
	id, ok := httpx.PathID(c)
	if !ok {
		return
	}
	detail, err := h.svc.GetWave(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, detail)
}

func (h *Handler) createWave(c *gin.Context) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 64 {
		response.Fail(c, errcode.ParamError)
		return
	}
	var req dto.CreateWaveReq
	if !httpx.BindJSON(c, &req) {
		return
	}
	ack, err := h.svc.CreateWave(c.Request.Context(), &req, middleware.Username(c), key)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, ack)
}

func (h *Handler) waveAction(action string) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if key == "" || len(key) > 64 {
			response.Fail(c, errcode.ParamError)
			return
		}
		id, ok := httpx.PathID(c)
		if !ok {
			return
		}
		var part int64
		if action == "collect" || action == "sort" || action == "ship" {
			var err error
			part, err = strconv.ParseInt(c.Param("part"), 10, 64)
			if err != nil || part <= 0 {
				response.Fail(c, errcode.ParamError)
				return
			}
		}
		var req dto.WaveQuantityReq
		if (action == "collect" || action == "sort") && !httpx.BindJSON(c, &req) {
			return
		}
		var ack *dto.WaveAck
		var err error
		ctx := c.Request.Context()
		operator := middleware.Username(c)
		switch action {
		case "claim":
			ack, err = h.svc.ClaimWave(ctx, id, operator, key)
		case "cancel":
			ack, err = h.svc.CancelWave(ctx, id, operator, key)
		case "collect":
			ack, err = h.svc.CollectWave(ctx, id, part, req, operator, key)
		case "sort":
			ack, err = h.svc.SortWave(ctx, id, part, req, operator, key)
		case "ship":
			ack, err = h.svc.ShipWaveOrder(ctx, id, part, operator, key)
		}
		if err != nil {
			response.Fail(c, err)
			return
		}
		response.OK(c, ack)
	}
}
