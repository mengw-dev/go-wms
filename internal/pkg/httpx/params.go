package httpx

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

// PathID 读取并校验当前路由的 :id 参数。失败时已经写入响应，调用方只需 return。
func PathID(c *gin.Context) (int64, bool) {
	id, ok := ID(c.Param("id"))
	if !ok {
		response.Fail(c, errcode.ParamError)
	}
	return id, ok
}

// QueryID 读取并校验正整数查询参数。
func QueryID(c *gin.Context, name string) (int64, bool) {
	id, ok := ID(c.Query(name))
	if !ok {
		response.Fail(c, errcode.ParamError)
	}
	return id, ok
}

// ID 解析正整数 ID。
func ID(value string) (int64, bool) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

// OptionalQueryID 解析可选正整数查询参数，空值返回 0。
func OptionalQueryID(c *gin.Context, name string) (int64, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, true
	}
	id, ok := ID(raw)
	if !ok {
		response.Fail(c, errcode.ParamError)
	}
	return id, ok
}

// QueryInt 解析查询整数并限制范围。
func QueryInt(c *gin.Context, name string, defaultValue, min, max int) (int, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return defaultValue, true
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		response.Fail(c, errcode.ParamError)
		return 0, false
	}
	return value, true
}

// IsBodyTooLarge 判断错误是否来自 http.MaxBytesReader。
func IsBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

// BindJSON 绑定 JSON 请求体，并区分参数错误和请求体过大。
func BindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			response.Fail(c, errcode.PayloadTooLarge)
			return false
		}
		response.Fail(c, errcode.ParamError)
		return false
	}
	return true
}

// FailParam 统一返回参数错误，避免 Handler 重复拼装响应。
func FailParam(c *gin.Context) {
	response.Fail(c, errcode.ParamError)
}
