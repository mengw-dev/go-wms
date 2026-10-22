package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type wavePermission bool

func (allowed wavePermission) HasPerm(context.Context, int64, string) bool { return bool(allowed) }

func TestWaveHTTPRejectsMissingKeyAndUnauthorizedWrites(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, allowed := range []bool{true, false} {
		for _, path := range []string{"/outbound/waves", "/outbound/waves/1/claim", "/outbound/waves/1/cancel", "/outbound/waves/1/lines/2/collect", "/outbound/waves/1/items/2/sort", "/outbound/waves/1/orders/2/ship"} {
			t.Run(path+map[bool]string{true: "/no-key", false: "/forbidden"}[allowed], func(t *testing.T) {
				router := gin.New()
				New(nil).registerWaves(router.Group("/outbound"), wavePermission(allowed))
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
				req.Header.Set("Content-Type", "application/json")
				if !allowed {
					req.Header.Set("Idempotency-Key", "key")
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, req)
				want := http.StatusBadRequest
				if !allowed {
					want = http.StatusForbidden
				}
				if response.Code != want {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
}
