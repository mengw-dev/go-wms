package response

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExcelDownloadHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	Excel(c, "inventory.xlsx", []byte("PK-test"))
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "spreadsheetml") || w.Header().Get("Content-Disposition") != "attachment; filename=inventory.xlsx" || w.Header().Get("Cache-Control") != "private, no-store" || w.Body.String() != "PK-test" {
		t.Fatalf("%d %v %s", w.Code, w.Header(), w.Body.String())
	}
}
