package dto

import (
	"testing"

	"github.com/gin-gonic/gin/binding"
)

func TestLoginTenantBinding(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload string
		valid   bool
		present bool
		id      int64
	}{
		{"omitted", `{"username":"tester","password":"secret"}`, true, false, 0},
		{"null", `{"username":"tester","password":"secret","tenant_id":null}`, true, false, 0},
		{"platform", `{"username":"tester","password":"secret","tenant_id":"0"}`, true, true, 0},
		{"large ID", `{"username":"tester","password":"secret","tenant_id":"9007199254740993"}`, true, true, 9007199254740993},
		{"negative", `{"username":"tester","password":"secret","tenant_id":"-1"}`, false, false, 0},
		{"overflow", `{"username":"tester","password":"secret","tenant_id":"9223372036854775808"}`, false, false, 0},
		{"not an integer", `{"username":"tester","password":"secret","tenant_id":"1.5"}`, false, false, 0},
		{"numeric ID is not the wire format", `{"username":"tester","password":"secret","tenant_id":11}`, false, false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var req LoginReq
			err := binding.JSON.BindBody([]byte(tt.payload), &req)
			if (err == nil) != tt.valid {
				t.Fatalf("binding error=%v valid=%v", err, tt.valid)
			}
			if tt.valid && ((req.TenantID != nil) != tt.present || (req.TenantID != nil && *req.TenantID != tt.id)) {
				t.Fatalf("wrong tenant field: %+v", req)
			}
		})
	}
}
