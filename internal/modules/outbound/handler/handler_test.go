package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestPDAPickRequiresIdempotencyKey PDA 入口必须携带 Idempotency-Key：
// 缺失（含纯空白）时在 handler 层直接 400；service 传 nil，
// 一旦请求穿透到业务层测试会直接失败。
func TestPDAPickRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name string
		key  string
	}{
		{name: "missing", key: ""},
		{name: "blank", key: "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPost, "/pda/tasks/1/pick",
				strings.NewReader(`{"qty":1,"location_code":"L1"}`))
			context.Request.Header.Set("Content-Type", "application/json")
			if c.key != "" {
				context.Request.Header.Set("Idempotency-Key", c.key)
			}
			context.Params = gin.Params{{Key: "id", Value: "1"}}

			New(nil).pickTask(context, true)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
