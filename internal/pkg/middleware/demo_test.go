package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/pkg/errcode"
)

type demoValidatorStub struct {
	enabled  bool
	username string
	err      error
}

func (s demoValidatorStub) Enabled() bool                                     { return s.enabled }
func (s demoValidatorStub) Username() string                                  { return s.username }
func (s demoValidatorStub) ValidateSession(_ context.Context, _ string) error { return s.err }

func setDemoTestUsername(username string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(string(ctxUsername), username)
		c.Next()
	}
}

func TestDemoSessionRequiresSessionForDemoUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setDemoTestUsername("demo"))
	router.Use(DemoSession(demoValidatorStub{enabled: true, username: "demo"}))
	router.GET("/api/v1/business", func(c *gin.Context) {
		c.Set(string(ctxUsername), "demo")
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/business", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusLocked {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusLocked)
	}
	if !strings.Contains(recorder.Body.String(), `"code":70003`) {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestDemoSessionAllowsAcquireWithoutSessionHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setDemoTestUsername("demo"))
	router.Use(DemoSession(demoValidatorStub{enabled: true, username: "demo"}))
	router.POST("/api/v1/demo/session/acquire", func(c *gin.Context) {
		c.Set(string(ctxUsername), "demo")
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "/api/v1/demo/session/acquire", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestDemoSessionPassesValidSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setDemoTestUsername("demo"))
	router.Use(DemoSession(demoValidatorStub{enabled: true, username: "demo"}))
	router.GET("/api/v1/business", func(c *gin.Context) {
		c.Set(string(ctxUsername), "demo")
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/business", nil)
	request.Header.Set("X-Demo-Session", "session-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestDemoSessionLeavesNonDemoUsersUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setDemoTestUsername("admin"))
	router.Use(DemoSession(demoValidatorStub{enabled: true, username: "demo"}))
	router.GET("/api/v1/business", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/business", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestDemoSessionLeavesDisabledDemoUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setDemoTestUsername("demo"))
	router.Use(DemoSession(demoValidatorStub{enabled: false, username: "demo"}))
	router.GET("/api/v1/business", func(c *gin.Context) {
		c.Set(string(ctxUsername), "demo")
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/business", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestDemoSessionMapsInvalidSessionError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(setDemoTestUsername("demo"))
	router.Use(DemoSession(demoValidatorStub{enabled: true, username: "demo", err: errcode.DemoSessionInvalid}))
	router.GET("/api/v1/business", func(c *gin.Context) {
		c.Set(string(ctxUsername), "demo")
		c.Status(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/business", nil)
	request.Header.Set("X-Demo-Session", "session-1")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusLocked {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusLocked)
	}
}
