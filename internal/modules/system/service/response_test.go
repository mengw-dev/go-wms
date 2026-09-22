package service

import (
	"encoding/json"
	"testing"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/typex"
)

func TestSystemResponsesOnlyExposePublicFields(t *testing.T) {
	user := &model.SysUser{
		Base:     model.Base{ID: 42},
		TenantID: 11,
		Username: "alice",
		Nickname: "Alice",
		Status:   1,
		RoleIDs:  typex.Int64List{10},
	}
	userResp := userResponse(user)
	if userResp.ID != user.ID || userResp.TenantID != user.TenantID || userResp.Username != user.Username {
		t.Fatalf("user fields changed: %+v", userResp)
	}

	role := &model.SysRole{
		Base:     model.Base{ID: 10},
		TenantID: 11,
		Name:     "operator",
		Perms:    "wms:inventory",
		Remark:   "operator role",
	}
	roleResp := roleResponse(role)
	if roleResp.ID != role.ID || roleResp.TenantID != role.TenantID || roleResp.Name != role.Name {
		t.Fatalf("role fields changed: %+v", roleResp)
	}

	for name, value := range map[string]any{"user": userResp, "role": roleResp} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %s response: %v", name, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("unmarshal %s response: %v", name, err)
		}
		for _, forbidden := range []string{"password_hash", "token_version", "deleted_at"} {
			if _, exists := fields[forbidden]; exists {
				t.Fatalf("%s response exposes %q: %s", name, forbidden, raw)
			}
		}
	}
}

func TestOperLogResponseKeepsAuditFieldsAndStringIDs(t *testing.T) {
	log := &model.SysOperLog{
		ID: 7, TenantID: 11, UserID: 42, Username: "alice", Path: "/users", Method: "POST",
		Params: `{"name":"Alice"}`, IP: "127.0.0.1", CostMs: 12, Status: 200, Result: `{"code":0}`,
	}
	resp := &dto.OperLogResp{
		ID: log.ID, TenantID: log.TenantID, UserID: log.UserID, Username: log.Username,
		Path: log.Path, Method: log.Method, Params: log.Params, IP: log.IP,
		CostMs: log.CostMs, Status: log.Status, Result: log.Result, CreatedAt: log.CreatedAt,
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal oper log response: %v", err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal oper log response: %v", err)
	}
	if fields["id"] != "7" || fields["user_id"] != "42" {
		t.Fatalf("oper log response ID contract changed: %s", raw)
	}
	if fields["path"] != "/users" || fields["status"] != float64(200) {
		t.Fatalf("oper log response fields changed: %s", raw)
	}
}
