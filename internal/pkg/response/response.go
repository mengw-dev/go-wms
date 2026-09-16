package response

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
)

// Body 统一响应结构 {code, msg, data}。
type Body struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

type pageData struct {
	List  any   `json:"list"`
	Total int64 `json:"total"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Body{Code: 0, Msg: "success", Data: data})
}

func OKPage(c *gin.Context, list any, total int64) {
	c.JSON(http.StatusOK, Body{Code: 0, Msg: "success", Data: pageData{List: list, Total: total}})
}

// Fail 将 error 归一化后输出；*errcode.Error 使用其 code/msg，其他按 500 处理。
// HTTP 状态码按业务错误码语义映射（非固定 200）：
// 401 未登录、403 无权限、409 并发冲突、500 系统错误，其余业务错误返回 400。
// 前端依赖 HTTP 401 触发登录失效（清 token 跳登录页），网关/监控依赖非 2xx 感知异常。
func Fail(c *gin.Context, err error) {
	var e *errcode.Error
	if !errors.As(err, &e) {
		e = errcode.Internal
	}
	if e.Code == errcode.Internal.Code {
		log.WithContext(c.Request.Context()).Error("request failed",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"err", err,
		)
	}
	c.JSON(httpStatus(e.Code), Body{Code: e.Code, Msg: e.Msg, Data: nil})
}

// httpStatus 业务错误码 → HTTP 状态码映射。
func httpStatus(code int) int {
	switch code {
	case errcode.Unauthorized.Code:
		return http.StatusUnauthorized
	case errcode.Forbidden.Code:
		return http.StatusForbidden
	case errcode.IntegrationUnauthorized.Code:
		return http.StatusUnauthorized
	case errcode.IntegrationDisabled.Code:
		return http.StatusServiceUnavailable
	case errcode.Internal.Code:
		return http.StatusInternalServerError
	case errcode.PayloadTooLarge.Code:
		return http.StatusRequestEntityTooLarge
	case errcode.Conflict.Code:
		return http.StatusConflict
	}
	if errcode.IsConflictCode(code) { // 各模块乐观锁/行竞争冲突
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
