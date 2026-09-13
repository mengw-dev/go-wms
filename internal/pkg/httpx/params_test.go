package httpx

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestID(t *testing.T) {
	if value, ok := ID("123"); !ok || value != 123 {
		t.Fatalf("ID(123) = %d, %v", value, ok)
	}
	for _, value := range []string{"", "0", "-1", "abc"} {
		if _, ok := ID(value); ok {
			t.Fatalf("ID(%q) should fail", value)
		}
	}
}

func TestQueryInt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/?page=3", nil)
	value, ok := QueryInt(ctx, "page", 1, 1, 100)
	if !ok || value != 3 {
		t.Fatalf("QueryInt(page) = %d, %v", value, ok)
	}

	ctx, _ = gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("GET", "/?page=0", nil)
	if _, ok := QueryInt(ctx, "page", 1, 1, 100); ok {
		t.Fatal("page=0 should fail validation")
	}
}
