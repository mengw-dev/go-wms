package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server      ServerConfig      `mapstructure:"server"`
	MySQL       MySQLConfig       `mapstructure:"mysql"`
	Redis       RedisConfig       `mapstructure:"redis"`
	JWT         JWTConfig         `mapstructure:"jwt"`
	Log         LogConfig         `mapstructure:"log"`
	Upload      UploadConfig      `mapstructure:"upload"`
	Metrics     MetricsConfig     `mapstructure:"metrics"`
	Integration IntegrationConfig `mapstructure:"integration"`
	Demo        DemoConfig        `mapstructure:"demo"`
	Personal    PersonalConfig    `mapstructure:"personal"`
	Limits      LimitsConfig      `mapstructure:"limits"`
	AI          AIConfig          `mapstructure:"ai"`
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

type IntegrationConfig struct {
	APIKey   string `mapstructure:"api_key"`
	TenantID int64  `mapstructure:"tenant_id"` // API Key 只访问这个租户；0 也精确隔离。
}

// DemoConfig 演示模块配置：多演示账号（demo1..demoN），每个账号独占一个租户，
// 数据互不影响；会话锁/数据重置均按租户隔离，退出（或超时）后该账号数据自动重置。
type DemoConfig struct {
	Enabled           bool   `mapstructure:"enabled"`
	Instances         int    `mapstructure:"instances"` // 演示账号数量（默认 5，最大 99）
	Password          string `mapstructure:"password"`
	SessionTTLSeconds int    `mapstructure:"session_ttl_seconds"`
}

// demoTenantIDBase 演示租户固定号段起点：第 i 个演示账号租户 = 10000+i。
// 与手工维护的业务租户（建议从 1 开始）错开，避免撞号。
const demoTenantIDBase int64 = 10000

// AccountUsername 第 i 个演示账号的用户名（i 从 1 开始，如 demo1、demo2）。
func (d DemoConfig) AccountUsername(i int) string { return fmt.Sprintf("demo%d", i) }

// AccountTenantID 第 i 个演示账号的租户 ID（10001、10002...）。
func (d DemoConfig) AccountTenantID(i int) int64 { return demoTenantIDBase + int64(i) }

// AccountIndex 由租户 ID 反查演示账号序号；非演示租户（含平台/业务租户）返回 0。
func (d DemoConfig) AccountIndex(tenantID int64) int {
	i := int(tenantID - demoTenantIDBase)
	if i < 1 || i > d.Instances {
		return 0
	}
	return i
}

// PersonalConfig 持久体验账号（user1..userN）：与演示账号同构——每个账号独占一个租户、
// 数据互不影响、共用固定密码；区别是数据长期保留（不参与演示重置），
// 且登录页由访客自行挑选账号，而不是像演示账号那样自动随机分配。
type PersonalConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Instances int    `mapstructure:"instances"` // 账号数量（默认 3，最大 99）
	Password  string `mapstructure:"password"`  // 所有持久账号共用密码（公开体验账号）
}

// personalTenantIDBase 持久账号租户号段起点：第 i 个账号租户 = 20000+i。
// 与演示账号（10001+）、手工维护的业务租户（建议从 1 开始）错开，避免撞号。
const personalTenantIDBase int64 = 20000

// AccountUsername 第 i 个持久账号的用户名（user1、user2...）。
func (p PersonalConfig) AccountUsername(i int) string { return fmt.Sprintf("user%d", i) }

// AccountTenantID 第 i 个持久账号的租户 ID（20001、20002...）。
func (p PersonalConfig) AccountTenantID(i int) int64 { return personalTenantIDBase + int64(i) }

// AccountNickname 第 i 个持久账号的昵称。
func (p PersonalConfig) AccountNickname(i int) string { return fmt.Sprintf("个人体验%d", i) }

// AccountIndexByUsername 由用户名反查持久账号序号；非持久账号返回 0。
func (p PersonalConfig) AccountIndexByUsername(username string) int {
	for i := 1; i <= p.Instances; i++ {
		if username == p.AccountUsername(i) {
			return i
		}
	}
	return 0
}

// LimitsConfig 公开租户（演示/持久账号）的数据量配额：防止访客无限写入撑爆数据库。
// 仅对租户 ID > 0 的租户生效；平台租户（tenant_id=0，如 admin）不受限。
type LimitsConfig struct {
	MaxReceiptOrders   int `mapstructure:"max_receipt_orders"`   // 每租户入库单上限
	MaxShipmentOrders  int `mapstructure:"max_shipment_orders"`  // 每租户出库单上限
	MaxStocktakeOrders int `mapstructure:"max_stocktake_orders"` // 每租户盘点单上限
	MaxSKUs            int `mapstructure:"max_skus"`             // 每租户货品上限
	MaxWarehouses      int `mapstructure:"max_warehouses"`       // 每租户仓库上限
	MaxLocations       int `mapstructure:"max_locations"`        // 每租户库位上限
	MaxImportRows      int `mapstructure:"max_import_rows"`      // 单次 Excel 导入行数上限
}

// AIConfig AI 库存问答（智谱 BigModel，OpenAI 兼容接口）。
// APIKey 只从环境变量 ZHIPU_API_KEY 读取，绝不写入配置文件；
// 模型名可被 ZHIPU_LLM_MODEL / ZHIPU_LLM_BACKUP_MODEL 环境变量覆盖。
type AIConfig struct {
	APIKey          string `mapstructure:"-"`
	Model           string `mapstructure:"model"`        // 主模型（优先最强免费模型）
	BackupModel     string `mapstructure:"backup_model"` // 备用模型：主模型拥堵(1305/429)时自动降级
	TimeoutSeconds  int    `mapstructure:"timeout_seconds"`
	BaseURL         string `mapstructure:"base_url"`
	RateLimitPerMin int    `mapstructure:"rate_limit_per_min"` // 每用户每分钟提问上限（Redis 计数）
	// DailyLimitPerTenant 每租户每日提问上限：共享体验账号下防止单个访客刷爆当天额度；
	// 仅对租户 ID > 0 生效（平台租户不限），Redis 不可用时降级放行。
	DailyLimitPerTenant int `mapstructure:"daily_limit_per_tenant"`
}

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
