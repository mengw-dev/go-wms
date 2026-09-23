package handler

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/demo/service"
	"gowms/internal/pkg/errcode"
)

func TestWriteScenarioFailureReturnsMergedExecutionResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	merged := &service.ScenarioResult{
		Name:    service.ScenarioFull,
		Summary: "部分业务已完成",
		Status:  service.ScenarioStatusFailed,
		Steps: []service.ScenarioStep{{
			Title:  "创建入库单",
			Detail: "已完成",
			Status: service.ScenarioStepCompleted,
		}},
	}
	err := &service.ScenarioExecutionError{
		Result: &service.ScenarioResult{Name: service.ScenarioOutbound},
		Err:    errcode.ShipOrderStatusWrong,
	}

	writeScenarioFailure(c, merged, err)

	if recorder.Code != 400 {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
	var body struct {
		Code int                    `json:"code"`
		Data service.ScenarioResult `json:"data"`
	}
	if decodeErr := json.Unmarshal(recorder.Body.Bytes(), &body); decodeErr != nil {
		t.Fatalf("decode response: %v", decodeErr)
	}
	if body.Code != errcode.ShipOrderStatusWrong.Code {
		t.Fatalf("code = %d, want %d", body.Code, errcode.ShipOrderStatusWrong.Code)
	}
	if body.Data.Name != service.ScenarioFull || len(body.Data.Steps) != 1 {
		t.Fatalf("response data = %#v", body.Data)
	}
}
