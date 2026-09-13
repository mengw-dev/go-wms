package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadRejectsUnsafeReleaseConfig(t *testing.T) {
	path := writeConfig(t, `server:
  port: 8080
  mode: release
  node: 1
jwt:
  secret: short
  expire_hours: 24
`)
	if _, err := Load(path); err == nil {
		t.Fatal("short release JWT secret should be rejected")
	}
}

func TestLoadRejectsInvalidNode(t *testing.T) {
	path := writeConfig(t, `server:
  port: 8080
  mode: debug
  node: 1024
jwt:
  secret: dev-secret
  expire_hours: 24
`)
	if _, err := Load(path); err == nil {
		t.Fatal("node above 1023 should be rejected")
	}
}

func TestLoadReadsEnvironmentOverrides(t *testing.T) {
	t.Setenv("WMS_SERVER_MODE", "release")
	t.Setenv("WMS_SERVER_NODE", "9")
	t.Setenv("WMS_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	cfg, err := Load(filepath.Join("..", "..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatalf("load config with env: %v", err)
	}
	if cfg.Server.Mode != "release" || cfg.Server.Node != 9 || cfg.JWT.Secret != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("environment overrides were not applied: %+v", cfg)
	}
}

func TestLoadValidConfig(t *testing.T) {
	path := writeConfig(t, `server:
  port: 8080
  mode: release
  node: 3
jwt:
  secret: 0123456789abcdef0123456789abcdef
  expire_hours: 24
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.Node != 3 || cfg.MySQL.MaxOpenConns != 50 || cfg.MySQL.MaxIdleConns != 10 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}
