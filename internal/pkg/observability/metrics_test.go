package observability

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMetricsMiddlewareRecordsRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	metrics := New(nil, "test-service")
	router := gin.New()
	router.Use(metrics.Middleware())
	router.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/ping", nil))
	if response.Code != 200 {
		t.Fatalf("unexpected status: %d", response.Code)
	}

	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() == "wms_http_requests_total" {
			return
		}
	}
	t.Fatal("wms_http_requests_total metric not found")
}
