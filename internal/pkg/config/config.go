package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	MySQL   MySQLConfig   `mapstructure:"mysql"`
	Redis   RedisConfig   `mapstructure:"redis"`
	JWT     JWTConfig     `mapstructure:"jwt"`
	Log     LogConfig     `mapstructure:"log"`
	Upload  UploadConfig  `mapstructure:"upload"`
	Metrics MetricsConfig `mapstructure:"metrics"`
}

type ServerConfig struct {
	Port                   int    `mapstructure:"port"`
	Mode                   string `mapstructure:"mode"` // debug / release
	Node                   int64  `mapstructure:"node"` // 雪花算法节点号（0-1023），多实例部署时每实例必须唯一
	ReadTimeoutSeconds     int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds    int    `mapstructure:"write_timeout_seconds"`
	IdleTimeoutSeconds     int    `mapstructure:"idle_timeout_seconds"`
	ShutdownTimeoutSeconds int    `mapstructure:"shutdown_timeout_seconds"`
	BodyLimitMB            int64  `mapstructure:"body_limit_mb"`
	CORSAllowOrigins       string `mapstructure:"cors_allow_origins"`
	TrustedProxies         string `mapstructure:"trusted_proxies"`
}

type MySQLConfig struct {
	DSN          string `mapstructure:"dsn"`
	MaxOpenConns int    `mapstructure:"max_open_conns"`
	MaxIdleConns int    `mapstructure:"max_idle_conns"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret      string `mapstructure:"secret"`
	ExpireHours int    `mapstructure:"expire_hours"`
}

type LogConfig struct {
	Level string `mapstructure:"level"` // debug / info / warn / error
}

type UploadConfig struct {
	Dir string `mapstructure:"dir"`
}

type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// Load 读取 configs/config.yaml；支持环境变量覆盖（WMS_ 前缀，. 分隔，如 WMS_MYSQL_DSN）。
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetEnvPrefix("WMS")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

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
	if cfg.Server.Node <= 0 {
		cfg.Server.Node = 1
	}
	if cfg.Server.Node > 1023 {
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
