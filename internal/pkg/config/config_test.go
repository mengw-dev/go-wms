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

func TestLoadNodeZeroAndNegative(t *testing.T) {
	for _, node := range []string{"0", "-1"} {
		t.Run(node, func(t *testing.T) {
			path := writeConfig(t, "server:\n  port: 8080\n  mode: debug\n  node: "+node+"\n")
			cfg, err := Load(path)
			if node == "-1" {
				if err == nil {
					t.Fatal("negative node accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Server.Node != 0 {
				t.Fatalf("node zero changed to %d", cfg.Server.Node)
			}
		})
	}
}

func TestLoadReadsEnvironmentOverrides(t *testing.T) {
	t.Setenv("WMS_SERVER_MODE", "release")
	t.Setenv("WMS_SERVER_NODE", "9")
	t.Setenv("WMS_JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("WMS_MYSQL_DSN", "produser:strongpass@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local")
	t.Setenv("WMS_INTEGRATION_API_KEY", "real-integration-key-for-prod")
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
mysql:
  dsn: "produser:strongpass@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
jwt:
  secret: 0123456789abcdef0123456789abcdef
  expire_hours: 24
integration:
  api_key: "real-integration-key-for-prod"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Server.Node != 3 || cfg.MySQL.MaxOpenConns != 50 || cfg.MySQL.MaxIdleConns != 10 {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadRejectsDevDSNInRelease(t *testing.T) {
	// CI 在 shell 里 export 了 WMS_MYSQL_DSN（用作集成测试 DSN），
	// viper 的 AutomaticEnv 会用它覆盖 yaml 中的 mysql.dsn，让 release 校验放行。
	// 这里把 WMS_MYSQL_DSN 显式钉到测试期望的 dev 值，确保测试对 ambient env 免疫。
	t.Setenv("WMS_MYSQL_DSN", "root:1234@tcp(127.0.0.1:3306)/gowms")
	path := writeConfig(t, `server:
  port: 8080
  mode: release
  node: 3
mysql:
  dsn: "root:1234@tcp(127.0.0.1:3306)/gowms"
jwt:
  secret: 0123456789abcdef0123456789abcdef
  expire_hours: 24
integration:
  api_key: "real-integration-key-for-prod"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("release mode with dev DSN (root:1234) should be rejected")
	}
}

func TestLoadRejectsDevAPIKeyInRelease(t *testing.T) {
	// 同 TestLoadRejectsDevDSNInRelease，避免 ambient WMS_INTEGRATION_API_KEY 覆盖 yaml。
	t.Setenv("WMS_INTEGRATION_API_KEY", "change-this-integration-api-key")
	path := writeConfig(t, `server:
  port: 8080
  mode: release
  node: 3
mysql:
  dsn: "produser:strongpass@tcp(127.0.0.1:3306)/gowms"
jwt:
  secret: 0123456789abcdef0123456789abcdef
  expire_hours: 24
integration:
  api_key: "change-this-integration-api-key"
`)
	if _, err := Load(path); err == nil {
		t.Fatal("release mode with dev integration api_key should be rejected")
	}
}
