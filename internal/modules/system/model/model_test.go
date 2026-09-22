package model

import (
	"encoding/json"
	"testing"
)

func TestVersionedJSONDoesNotExposeInternalVersion(t *testing.T) {
	value := struct {
		Versioned
		Name string `json:"name"`
	}{
		Versioned: Versioned{Version: 7},
		Name:      "order",
	}

	got, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal versioned value: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("unmarshal versioned value: %v", err)
	}
	if _, exists := fields["version"]; exists {
		t.Fatalf("internal version must not be exposed, got %s", got)
	}
	if fields["name"] != "order" {
		t.Fatalf("business field changed, got %s", got)
	}
}

func TestSysUserJSONDoesNotExposeAuthenticationSecrets(t *testing.T) {
	user := SysUser{
		PasswordHash: "hash-must-not-leak",
		TokenVersion: 9,
	}

	got, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}

	var fields map[string]any
	if err := json.Unmarshal(got, &fields); err != nil {
		t.Fatalf("unmarshal user: %v", err)
	}
	for _, field := range []string{"password_hash", "token_version", "deleted_at"} {
		if _, exists := fields[field]; exists {
			t.Fatalf("internal field %q must not be exposed, got %s", field, got)
		}
	}
}
