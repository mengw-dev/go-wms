package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/response"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

// RegisterRoutes 注册路由；pub 无需登录，auth 已挂 Auth 中间件。
func (h *Handler) RegisterRoutes(pub, auth *gin.RouterGroup, checker middleware.PermsChecker) {
	pub.POST("/login", h.login)

	auth.GET("/profile", h.profile)
	auth.PUT("/password", h.changePassword)

	users := auth.Group("/system/users")
	{
		users.GET("", middleware.Permission(checker, "wms:system:user"), h.listUsers)
		users.POST("", middleware.Permission(checker, "wms:system:user"), h.createUser)
		users.PUT("/:id", middleware.Permission(checker, "wms:system:user"), h.updateUser)
		users.DELETE("/:id", middleware.Permission(checker, "wms:system:user"), h.deleteUser)
		users.PUT("/:id/status", middleware.Permission(checker, "wms:system:user"), h.updateUserStatus)
		users.PUT("/:id/password", middleware.Permission(checker, "wms:system:user"), h.resetPassword)
	}

	roles := auth.Group("/system/roles")
	{
		roles.GET("/all", middleware.Permission(checker, "wms:system:role"), h.listAllRoles)
		roles.GET("", middleware.Permission(checker, "wms:system:role"), h.listRoles)
		roles.POST("", middleware.Permission(checker, "wms:system:role"), h.createRole)
		roles.PUT("/:id", middleware.Permission(checker, "wms:system:role"), h.updateRole)
		roles.DELETE("/:id", middleware.Permission(checker, "wms:system:role"), h.deleteRole)
	}

	auth.GET("/system/oper-logs", middleware.Permission(checker, "wms:system:log"), h.listOperLogs)
}

func (h *Handler) login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	resp, err := h.svc.Login(c.Request.Context(), &req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

func (h *Handler) profile(c *gin.Context) {
	resp, err := h.svc.Profile(c.Request.Context(), middleware.UserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

func (h *Handler) changePassword(c *gin.Context) {
	var req dto.ChangePwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), middleware.UserID(c), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) listUsers(c *gin.Context) {
	var q dto.UserListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListUsers(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) createUser(c *gin.Context) {
	var req dto.UserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.svc.CreateUser(c.Request.Context(), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) updateUser(c *gin.Context) {
	var req dto.UserUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.UpdateUser(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) updateUserStatus(c *gin.Context) {
	var req dto.StatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.UpdateUser(c.Request.Context(), id, &dto.UserUpdateReq{Status: req.Status}); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) deleteUser(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.DeleteUser(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) resetPassword(c *gin.Context) {
	var req dto.ResetPwdReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.ResetPassword(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) listRoles(c *gin.Context) {
	var q dto.RoleListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListRoles(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}

func (h *Handler) listAllRoles(c *gin.Context) {
	list, err := h.svc.ListAllRoles(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, list)
}

func (h *Handler) createRole(c *gin.Context) {
	var req dto.RoleCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	if err := h.svc.CreateRole(c.Request.Context(), &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) updateRole(c *gin.Context) {
	var req dto.RoleUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.UpdateRole(c.Request.Context(), id, &req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) deleteRole(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.DeleteRole(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, nil)
}

func (h *Handler) listOperLogs(c *gin.Context) {
	var q dto.OperLogQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Fail(c, errcode.ParamError)
		return
	}
	list, total, err := h.svc.ListOperLogs(c.Request.Context(), &q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OKPage(c, list, total)
}
