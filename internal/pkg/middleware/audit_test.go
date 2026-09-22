package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/tenant"
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

type auditRecorderStub struct {
	ctx    context.Context
	record OperLogRecord
}

func (s *auditRecorderStub) Record(ctx context.Context, record OperLogRecord) {
	s.ctx, s.record = ctx, record
}

func TestOperLogPreservesTenantAndRedactsQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &auditRecorderStub{}
	router := gin.New()
	router.Use(OperLog(recorder))
	router.DELETE("/resource", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodDelete, "/resource?token=private-value&page=1", nil)
	req = req.WithContext(tenant.WithTenant(req.Context(), 42))
	router.ServeHTTP(httptest.NewRecorder(), req)
	if tenant.FromContext(recorder.ctx) != 42 {
		t.Fatal("audit tenant was lost")
	}
	if strings.Contains(recorder.record.Params, "private-value") {
		t.Fatalf("query secret leaked: %s", recorder.record.Params)
	}
}

func TestOperLogRejectsTruncatedRequestBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(BodyLimit(1), OperLog(nil))
	called := false
	router.POST("/resource", func(c *gin.Context) { called = true; c.Status(http.StatusNoContent) })
	// 前缀是合法 JSON，截断后不能变成一个可执行的请求。
	req := httptest.NewRequest(http.MethodPost, "/resource", strings.NewReader(`{}`+strings.Repeat(" ", 1<<20)))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("status=%d called=%v", recorder.Code, called)
	}
}

func TestMalformedAuditInputDoesNotLeakSecrets(t *testing.T) {
	for _, tt := range []struct{ contentType, body string }{
		{"application/json", `{"password":"private-value",`},
		{"application/x-www-form-urlencoded", "password=private-value&bad=%zz"},
	} {
		if got := sanitizeOperLogParams(tt.contentType, tt.body); strings.Contains(got, "private-value") {
			t.Fatalf("secret leaked: %s", got)
		}
	}
}

func TestOperLogLargeResponseDoesNotChangeHTTPBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := &auditRecorderStub{}
	router := gin.New()
	router.Use(OperLog(recorder))
	body := strings.Repeat("x", 128<<10)
	router.POST("/export", func(c *gin.Context) { c.String(http.StatusOK, "%s", body) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/export", nil))
	if response.Body.String() != body {
		t.Fatal("response changed")
	}
	if recorder.record.Result != "[LARGE RESPONSE OMITTED]" {
		t.Fatalf("large response retained: %d bytes", len(recorder.record.Result))
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
