package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type alertPermissions struct{ read, manage bool }

func (p alertPermissions) HasPerm(_ context.Context, _ int64, perm string) bool {
	if perm == "wms:inventory:alert:manage" {
		return p.manage
	}
	return p.read
}

func TestAlertHTTPPermissionAndRequestValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, body string
		perms                    alertPermissions
		status                   int
	}{
		{"forbidden-read", "GET", "/inventory/alerts", "", alertPermissions{}, 403},
		{"read-only-options", "GET", "/inventory/alert-options?kind=sku", "", alertPermissions{read: true}, 403},
		{"invalid-option-kind", "GET", "/inventory/alert-options?kind=location", "", alertPermissions{manage: true}, 400},
		{"read-only-save", "PUT", "/inventory/alert-rules", `{}`, alertPermissions{read: true}, 403},
		{"read-only-delete", "DELETE", "/inventory/alert-rules/1", "", alertPermissions{read: true}, 403},
		{"invalid-state", "GET", "/inventory/alerts?state=UNKNOWN", "", alertPermissions{read: true}, 400},
		{"invalid-warehouse", "GET", "/inventory/alerts?warehouse_id=-1", "", alertPermissions{read: true}, 400},
		{"missing-enabled", "PUT", "/inventory/alert-rules", `{"warehouse_id":"1","sku_id":"1","min_available_qty":10}`, alertPermissions{manage: true}, 400},
		{"zero-threshold", "PUT", "/inventory/alert-rules", `{"warehouse_id":"1","sku_id":"1","min_available_qty":0,"enabled":true}`, alertPermissions{manage: true}, 400},
		{"overflow-threshold", "PUT", "/inventory/alert-rules", `{"warehouse_id":"1","sku_id":"1","min_available_qty":2147483648,"enabled":true}`, alertPermissions{manage: true}, 400},
		{"invalid-json", "PUT", "/inventory/alert-rules", `{"warehouse_id":`, alertPermissions{manage: true}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			New(nil).RegisterRoutes(r.Group(""), tc.perms)
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}
