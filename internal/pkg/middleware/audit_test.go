package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSanitizeOperLogParamsRedactsSensitiveJSON(t *testing.T) {
	input := `{"username":"admin","password":"secret","nested":{"token":"abc","value":1}}`
	got := sanitizeOperLogParams("application/json", input)
	if strings.Contains(got, "secret") || strings.Contains(got, "abc") {
		t.Fatalf("sensitive value leaked: %s", got)
	}
	if !strings.Contains(got, "[REDACTED]") || !strings.Contains(got, "username") {
		t.Fatalf("unexpected sanitized output: %s", got)
	}
}

func TestSanitizeOperLogParamsRedactsRawQuery(t *testing.T) {
	got := sanitizeOperLogParams("", "username=admin&token=secret&page=1")
	if strings.Contains(got, "secret") {
		t.Fatalf("token leaked: %s", got)
	}
}

func TestTruncateUTF8DoesNotBreakCharacters(t *testing.T) {
	got := truncateUTF8(strings.Repeat("中文", 10), 7)
	if !strings.HasPrefix(got, "中") {
		t.Fatalf("unexpected truncated value: %q", got)
	}
}

func TestCORSAllowsOnlyConfiguredOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS([]string{"http://localhost:5173"}))
	router.GET("/ping", func(c *gin.Context) { c.Status(204) })

	request := httptest.NewRequest("GET", "/ping", nil)
	request.Header.Set("Origin", "http://localhost:5173")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("allowed origin header missing: %v", response.Header())
	}

	request = httptest.NewRequest("GET", "/ping", nil)
	request.Header.Set("Origin", "https://evil.example")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unexpected allow-origin header for disallowed origin")
	}
}

func TestNormalizeRequestID(t *testing.T) {
	if got := normalizeRequestID("request-123"); got != "request-123" {
		t.Fatalf("valid request id changed: %q", got)
	}
	if got := normalizeRequestID("bad\nheader"); got != "" {
		t.Fatalf("unsafe request id accepted: %q", got)
	}
}
