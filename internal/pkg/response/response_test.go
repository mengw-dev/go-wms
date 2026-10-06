package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
)

// TestHTTPStatus 业务错误码 → HTTP 状态码映射：
// 401/403 必须准确（前端依赖 401 触发登录失效），冲突类 409，系统错误 500，业务错误 400。
func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		err  *errcode.Error
		want int
	}{
		{errcode.Unauthorized, 401},
		{errcode.Forbidden, 403},
		{errcode.Internal, 500},
		{errcode.PayloadTooLarge, 413},
		{errcode.Conflict, 409},
		{errcode.OrderVersionBad, 409},
		{errcode.ShipOrderVersionBad, 409},
		{errcode.AllocConflict, 409},
		{errcode.ShipConflict, 409},
		{errcode.StocktakeVersionBad, 409},
		{errcode.TaskClaimConflict, 409},
		{errcode.TaskClaimMismatch, 409},
		{errcode.TaskLeaseExpired, 409},
		{errcode.ParamError, 400},
		{errcode.AvailableNotEnough, 400},
		{errcode.OrderStatusWrong, 400},
		{errcode.UserOrPwdWrong, 400},
		{errcode.NotFound, 404},
		{errcode.WarehouseNotFound, 404},
		{errcode.SKUNotFound, 404},
		{errcode.OrderNotFound, 404},
		{errcode.ShipOrderNotFound, 404},
		{errcode.StocktakeNotFound, 404},
		{errcode.IntegrationUnauthorized, 401},
		{errcode.IntegrationDisabled, 503},
		{errcode.DemoDisabled, 503},
		{errcode.DemoDataMissing, 503},
		{errcode.DemoBusy, 423},
		{errcode.DemoSessionInvalid, 423},
	}
	for _, c := range cases {
		if got := httpStatus(c.err); got != c.want {
			t.Errorf("httpStatus(%d) = %d, want %d", c.err.Code, got, c.want)
		}
	}
}

func TestFailWithDataKeepsBusinessErrorAndPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/demo/run/full", nil)

	FailWithData(c, errcode.OrderStatusWrong, map[string]any{
		"status": "failed",
		"steps":  []map[string]any{{"title": "提交订单", "status": "failed"}},
	})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var body struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Code != errcode.OrderStatusWrong.Code || body.Msg != errcode.OrderStatusWrong.Msg {
		t.Fatalf("response code/message = %d/%q", body.Code, body.Msg)
	}
	if body.Data["status"] != "failed" {
		t.Fatalf("response data = %#v", body.Data)
	}
}
