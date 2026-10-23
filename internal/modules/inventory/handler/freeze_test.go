package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type freezePermission struct{ read, freeze bool }

func (p freezePermission) HasPerm(_ context.Context, _ int64, permission string) bool {
	if permission == "wms:inventory" {
		return p.read
	}
	if permission == "wms:inventory:freeze" {
		return p.freeze
	}
	return false
}
func TestFreezeHTTPRequiresPermissionAndStrictPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, body, key string
		perms           freezePermission
		status          int
	}{
		{"no freeze permission", `{}`, "k", freezePermission{read: true}, 403},
		{"no read permission", `{}`, "k", freezePermission{freeze: true}, 403},
		{"missing key", `{}`, "", freezePermission{true, true}, 400},
		{"zero quantity", `{"inventory_id":"1","quantity":0,"reason":"质量"}`, "k", freezePermission{true, true}, 400},
		{"overflow", `{"inventory_id":"1","quantity":2147483648,"reason":"质量"}`, "k", freezePermission{true, true}, 400},
		{"trailing JSON", `{"inventory_id":"1","quantity":1,"reason":"质量"}[]`, "k", freezePermission{true, true}, 400},
		{"unknown", `{"inventory_id":"1","quantity":1,"reason":"质量","tenant_id":22}`, "k", freezePermission{true, true}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			New(nil).RegisterRoutes(router.Group(""), tc.perms)
			req := httptest.NewRequest("POST", "/inventory/freeze", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", tc.key)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}
