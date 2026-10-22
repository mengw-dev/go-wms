package handler

import (
	"context"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"strings"
	"testing"
)

type movePermissions struct{ read, move bool }

func (p movePermissions) HasPerm(_ context.Context, _ int64, perm string) bool {
	if perm == "wms:inventory" {
		return p.read
	}
	if perm == "wms:inventory:move" {
		return p.move
	}
	return false
}
func TestMoveHTTPPermissionsAndValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, body string
		perms                    movePermissions
		status                   int
	}{
		{"no-read-history", "GET", "/inventory/moves", "", movePermissions{}, 403},
		{"no-move-write", "POST", "/inventory/moves", `{}`, movePermissions{read: true}, 403},
		{"no-read-write", "POST", "/inventory/moves", `{}`, movePermissions{move: true}, 403},
		{"no-move-source", "GET", "/inventory/move-source/1", "", movePermissions{read: true}, 403},
		{"no-move-options", "GET", "/inventory/move-locations?source_inventory_id=1", "", movePermissions{read: true}, 403},
		{"bad-page", "GET", "/inventory/moves?page=-1", "", movePermissions{read: true}, 400},
		{"unbounded-page", "GET", "/inventory/moves?page_size=101", "", movePermissions{read: true}, 400},
		{"bad-source", "GET", "/inventory/move-source/-1", "", movePermissions{true, true}, 400},
		{"missing-source-option", "GET", "/inventory/move-locations", "", movePermissions{true, true}, 400},
		{"missing-fields", "POST", "/inventory/moves", `{}`, movePermissions{true, true}, 400},
		{"zero-quantity", "POST", "/inventory/moves", `{"source_inventory_id":"1","target_location_id":"2","quantity":0,"reason":"整理"}`, movePermissions{true, true}, 400},
		{"overflow", "POST", "/inventory/moves", `{"source_inventory_id":"1","target_location_id":"2","quantity":2147483648,"reason":"整理"}`, movePermissions{true, true}, 400},
		{"invalid-json", "POST", "/inventory/moves", `{"source_inventory_id":`, movePermissions{true, true}, 400},
		{"trailing-json", "POST", "/inventory/moves", `{"source_inventory_id":"1","target_location_id":"2","quantity":1,"reason":"整理"}{}`, movePermissions{true, true}, 400},
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
