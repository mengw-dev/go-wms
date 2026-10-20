package handler

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

// 可选请求体仅允许真正的空输入或一个 JSON 对象，不以 ContentLength 推测是否有效。
func bindOptionalJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	var raw json.RawMessage
	if err := decoder.Decode(&raw); errors.Is(err, io.EOF) {
		return true
	} else if err != nil || len(raw) == 0 || raw[0] != '{' {
		response.Fail(c, errcode.ParamError)
		return false
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		response.Fail(c, errcode.ParamError)
		return false
	}
	if err := json.Unmarshal(raw, target); err != nil {
		response.Fail(c, errcode.ParamError)
		return false
	}
	if err := binding.Validator.ValidateStruct(target); err != nil {
		response.Fail(c, errcode.ParamError)
		return false
	}
	return true
}
