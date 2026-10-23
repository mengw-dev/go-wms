package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/response"
)

// CommandKey requires an explicit request identity before command execution.
func CommandKey(c *gin.Context) bool {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 64 {
		response.Fail(c, errcode.ParamError)
		return false
	}
	return true
}

// BindStrictJSON rejects unknown fields and trailing JSON for stock commands;
// a successful decode of only the first object must never trigger a mutation.
func BindStrictJSON(c *gin.Context, target any) bool {
	fail := func(e error) bool {
		if IsBodyTooLarge(e) {
			response.Fail(c, errcode.PayloadTooLarge)
		} else {
			response.Fail(c, errcode.ParamError)
		}
		return false
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if e := decoder.Decode(target); e != nil {
		return fail(e)
	}
	var extra any
	if e := decoder.Decode(&extra); !errors.Is(e, io.EOF) {
		return fail(e)
	}
	if e := binding.Validator.ValidateStruct(target); e != nil {
		return fail(e)
	}
	return true
}
