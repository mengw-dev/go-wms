package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		expected   string
		provided   string
		wantStatus int
	}{
		{name: "disabled", expected: "", provided: "anything", wantStatus: http.StatusServiceUnavailable},
		{name: "missing", expected: "secret-key", provided: "", wantStatus: http.StatusUnauthorized},
		{name: "wrong", expected: "secret-key", provided: "wrong-key", wantStatus: http.StatusUnauthorized},
		{name: "ok", expected: "secret-key", provided: "secret-key", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.POST("/integration", APIKey(tt.expected), func(c *gin.Context) {
				c.Status(http.StatusOK)
			})
			req := httptest.NewRequest(http.MethodPost, "/integration", nil)
			if tt.provided != "" {
				req.Header.Set("X-API-Key", tt.provided)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}
