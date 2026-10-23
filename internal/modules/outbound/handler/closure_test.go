package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type closurePermission struct{ read, write bool }

func (p closurePermission) HasPerm(_ context.Context, _ int64, permission string) bool {
	if permission == "wms:outbound:view" {
		return p.read
	}
	return p.write
}
func TestClosureHTTPRequiresPermissionKeyAndStrictPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, method, path, body, key string
		read, write                   bool
		status                        int
	}{
		{name: "no exception permission", method: "POST", path: "/outbound/exceptions", body: `{}`, read: true, status: 403},
		{name: "no return permission", method: "POST", path: "/outbound/returns", body: `{}`, read: true, status: 403},
		{name: "no abort permission", method: "POST", path: "/outbound/waves/1/abort", body: `{}`, read: true, status: 403},
		{name: "no read", method: "GET", path: "/outbound/returns", write: true, status: 403},
		{name: "no key", method: "POST", path: "/outbound/returns", body: `{}`, read: true, write: true, status: 400},
		{name: "trailing JSON", method: "POST", path: "/outbound/returns", body: `{"allocation_id":"1","location_id":"2","good_qty":1,"bad_qty":0,"reason":"退货"}{}`, key: "k", read: true, write: true, status: 400},
		{name: "unknown field", method: "POST", path: "/outbound/returns", body: `{"allocation_id":"1","location_id":"2","good_qty":1,"bad_qty":0,"reason":"退货","tenant_id":"22"}`, key: "k", read: true, write: true, status: 400},
		{name: "overflow qty", method: "POST", path: "/outbound/returns", body: `{"allocation_id":"1","location_id":"2","good_qty":2147483648,"reason":"退货"}`, key: "k", read: true, write: true, status: 400},
		{name: "bad page", method: "GET", path: "/outbound/exceptions?page_size=101", read: true, status: 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			New(nil).registerClosure(router.Group("/outbound"), closurePermission{tc.read, tc.write})
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Idempotency-Key", tc.key)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", response.Code, tc.status, response.Body.String())
			}
		})
	}
}
