// Package config 加载、覆盖并校验应用配置。
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Load 读取 configs/config.yaml；支持环境变量覆盖（WMS_ 前缀，. 分隔，如 WMS_MYSQL_DSN）。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("WMS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("integration.tenant_id", int64(0))

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if cfg.Server.Port <= 0 || cfg.Server.Port > 65535 {
		return nil, fmt.Errorf("server.port must be between 1 and 65535")
	}
	if cfg.Server.Mode != "debug" && cfg.Server.Mode != "release" {
		return nil, fmt.Errorf("server.mode must be debug or release")
	}
	if cfg.Integration.TenantID < 0 {
		return nil, fmt.Errorf("integration.tenant_id must be non-negative")
	}
	if cfg.Server.ReadTimeoutSeconds <= 0 {
		cfg.Server.ReadTimeoutSeconds = 15
	}
	if cfg.Server.WriteTimeoutSeconds <= 0 {
		cfg.Server.WriteTimeoutSeconds = 30
	}
	if cfg.Server.IdleTimeoutSeconds <= 0 {
		cfg.Server.IdleTimeoutSeconds = 60
	}
	if cfg.Server.ShutdownTimeoutSeconds <= 0 {
		cfg.Server.ShutdownTimeoutSeconds = 10
	}
	if cfg.Server.BodyLimitMB <= 0 {
		cfg.Server.BodyLimitMB = 10
	}
	if cfg.Server.CORSAllowOrigins == "" && cfg.Server.Mode == "debug" {
		cfg.Server.CORSAllowOrigins = "http://localhost:5173,http://127.0.0.1:5173"
	}
	if cfg.MySQL.MaxOpenConns <= 0 {
		cfg.MySQL.MaxOpenConns = 50
	}
	if cfg.MySQL.MaxIdleConns <= 0 {
		cfg.MySQL.MaxIdleConns = 10
	}
	if cfg.Server.Node < 0 || cfg.Server.Node > 1023 {
		return nil, fmt.Errorf("server.node must be between 0 and 1023")
	}
	if cfg.JWT.ExpireHours <= 0 {
		cfg.JWT.ExpireHours = 24
	}
	if cfg.Server.Mode == "release" {
		secret := strings.ToLower(cfg.JWT.Secret)
		if len(cfg.JWT.Secret) < 32 || strings.Contains(secret, "change") || strings.Contains(secret, "dev-secret") {
			return nil, fmt.Errorf("release mode requires a random WMS_JWT_SECRET with at least 32 characters")
		}
		// 拒绝开发默认 DSN（root:1234 或占位符），强制用 WMS_MYSQL_DSN 注入生产凭据
		if strings.Contains(cfg.MySQL.DSN, "root:1234") || strings.Contains(cfg.MySQL.DSN, "change-this") {
			return nil, fmt.Errorf("release mode requires WMS_MYSQL_DSN to override the dev default in config.yaml")
		}
		if cfg.Integration.APIKey == "" || strings.Contains(strings.ToLower(cfg.Integration.APIKey), "change-this") {
			return nil, fmt.Errorf("release mode requires WMS_INTEGRATION_API_KEY to override the dev default in config.yaml")
		}
	}
	if cfg.Upload.Dir == "" {
		cfg.Upload.Dir = "./data/uploads"
	}
	if cfg.Metrics.Port <= 0 || cfg.Metrics.Port > 65535 {
		cfg.Metrics.Port = 9090
	}
	if cfg.Metrics.Path == "" {
		cfg.Metrics.Path = "/metrics"
	}
	if !strings.HasPrefix(cfg.Metrics.Path, "/") {
		cfg.Metrics.Path = "/" + cfg.Metrics.Path
	}
	if cfg.Demo.Instances <= 0 {
		cfg.Demo.Instances = 5
	}
	if cfg.Demo.Instances > 99 {
		return nil, fmt.Errorf("demo.instances must be between 1 and 99")
	}
	if cfg.Demo.Password == "" {
		cfg.Demo.Password = "demo123456"
	}
	if cfg.Demo.SessionTTLSeconds <= 0 {
		cfg.Demo.SessionTTLSeconds = 300
	}
	// 持久体验账号（user1..userN）：与演示账号同构但数据不清零。
	if cfg.Personal.Instances <= 0 {
		cfg.Personal.Instances = 3
	}
	if cfg.Personal.Instances > 99 {
		return nil, fmt.Errorf("personal.instances must be between 1 and 99")
	}
	if cfg.Personal.Password == "" {
		cfg.Personal.Password = "user123456"
	}
	// 公开租户数据量配额：未配置时给保守默认值，防止访客无限写入。
	if cfg.Limits.MaxReceiptOrders <= 0 {
		cfg.Limits.MaxReceiptOrders = 200
	}
	if cfg.Limits.MaxShipmentOrders <= 0 {
		cfg.Limits.MaxShipmentOrders = 200
	}
	if cfg.Limits.MaxStocktakeOrders <= 0 {
		cfg.Limits.MaxStocktakeOrders = 100
	}
	if cfg.Limits.MaxSKUs <= 0 {
		cfg.Limits.MaxSKUs = 500
	}
	if cfg.Limits.MaxWarehouses <= 0 {
		cfg.Limits.MaxWarehouses = 20
	}
	if cfg.Limits.MaxLocations <= 0 {
		cfg.Limits.MaxLocations = 500
	}
	if cfg.Limits.MaxImportRows <= 0 {
		cfg.Limits.MaxImportRows = 200
	}
	// AI 段：密钥只走环境变量（ZHIPU_ 前缀与 viper 的 WMS_ 前缀不同，需单独读取）
	cfg.AI.APIKey = strings.TrimSpace(os.Getenv("ZHIPU_API_KEY"))
	if v := strings.TrimSpace(os.Getenv("ZHIPU_LLM_MODEL")); v != "" {
		cfg.AI.Model = v
	}
	if v := strings.TrimSpace(os.Getenv("ZHIPU_LLM_BACKUP_MODEL")); v != "" {
		cfg.AI.BackupModel = v
	}
	if cfg.AI.Model == "" {
		cfg.AI.Model = "glm-4.7-flash"
	}
	if cfg.AI.BackupModel == "" {
		cfg.AI.BackupModel = "glm-4-flash-250414"
	}
	if cfg.AI.BaseURL == "" {
		cfg.AI.BaseURL = "https://open.bigmodel.cn/api/paas/v4"
	}
	if cfg.AI.TimeoutSeconds <= 0 {
		cfg.AI.TimeoutSeconds = 30
	}
	if cfg.AI.RateLimitPerMin <= 0 {
		cfg.AI.RateLimitPerMin = 10
	}
	if cfg.AI.DailyLimitPerTenant <= 0 {
		cfg.AI.DailyLimitPerTenant = 100
	}
	return &cfg, nil
}

func SplitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if item := strings.TrimSpace(part); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func (c *Config) CORSOrigins() []string       { return SplitCSV(c.Server.CORSAllowOrigins) }
func (c *Config) TrustedProxyCIDRs() []string { return SplitCSV(c.Server.TrustedProxies) }
