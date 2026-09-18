package handler

import (
	"github.com/gin-gonic/gin"

	"gowms/internal/modules/ai/dto"
	"gowms/internal/modules/ai/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes AI 问答路由；权限复用 wms:inventory 读权限（能看库存即可问库存）。
func (h *Handler) RegisterRoutes(auth *gin.RouterGroup, checker middleware.PermsChecker) {
	g := auth.Group("/ai")
	read := middleware.Permission(checker, "wms:inventory")
	g.POST("/chat", read, h.chat)
}

func (h *Handler) chat(c *gin.Context) {
	var req dto.ChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	answer, err := h.svc.Chat(c.Request.Context(), middleware.UserID(c), req.Question)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, dto.ChatResp{Answer: answer})
}
