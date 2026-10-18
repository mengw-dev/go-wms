package dto

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLocationBatchRespJSONContract(t *testing.T) {
	raw, err := json.Marshal(LocationBatchResp{Created: 3})
	if err != nil {
		t.Fatalf("marshal location batch response: %v", err)
	}
	if string(raw) != `{"created":3}` {
		t.Fatalf("unexpected location batch response: %s", raw)
	}
}

func TestLocationBatchCoordinateBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		value string
		valid bool
	}{
		{"1", true}, {"2147483647", true}, {"0", false}, {"-1", false}, {"2147483648", false}, {"9223372036854775807", false},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		body := `{"warehouse_id":"1","zone":"A","row_from":` + test.value + `,"row_to":` + test.value + `,"col_from":1,"col_to":1}`
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		var request LocationBatchReq
		if err := c.ShouldBindJSON(&request); (err == nil) != test.valid {
			t.Fatalf("coordinate=%s err=%v", test.value, err)
		}
	}
}
