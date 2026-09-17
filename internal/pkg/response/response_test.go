package response

import (
	"testing"

	"gowms/internal/pkg/errcode"
)

// TestHTTPStatus 业务错误码 → HTTP 状态码映射：
// 401/403 必须准确（前端依赖 401 触发登录失效），冲突类 409，系统错误 500，业务错误 400。
func TestHTTPStatus(t *testing.T) {
	cases := []struct {
		code int
		want int
	}{
		{errcode.Unauthorized.Code, 401},       // 40100 登录失效
		{errcode.Forbidden.Code, 403},          // 40300 无权限
		{errcode.Internal.Code, 500},           // 500 系统错误
		{errcode.PayloadTooLarge.Code, 413},    // 413 请求体过大
		{errcode.Conflict.Code, 409},           // 40900 通用并发冲突
		{40003, 409},                           // 入库单版本冲突
		{50003, 409},                           // 出库单版本冲突
		{50201, 409},                           // 分配并发冲突
		{50202, 409},                           // 发货并发冲突
		{60005, 409},                           // 盘点单版本冲突
		{errcode.ParamError.Code, 400},         // 参数错误
		{errcode.AvailableNotEnough.Code, 400}, // 30201 业务规则
		{errcode.OrderStatusWrong.Code, 400},   // 40002 状态机
		{errcode.UserOrPwdWrong.Code, 400},     // 10002 登录失败不是 401（未携带凭据/凭据错误）
		{errcode.IntegrationUnauthorized.Code, 401},
		{errcode.IntegrationDisabled.Code, 503},
		{errcode.DemoDisabled.Code, 503},
		{errcode.DemoDataMissing.Code, 503},
		{errcode.DemoBusy.Code, 423},
		{errcode.DemoSessionInvalid.Code, 423},
	}
	for _, c := range cases {
		if got := httpStatus(c.code); got != c.want {
			t.Errorf("httpStatus(%d) = %d, want %d", c.code, got, c.want)
		}
	}
}
