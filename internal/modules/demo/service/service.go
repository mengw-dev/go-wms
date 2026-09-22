package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	inboundservice "gowms/internal/modules/inbound/service"
	outboundservice "gowms/internal/modules/outbound/service"
	stocktakeservice "gowms/internal/modules/stocktake/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// 演示会话锁按租户隔离：每个演示账号（demo1..demoN）独占一个租户，
// 同一时刻一个租户只允许一个访客操作，互不干扰。
func activeSessionKeyOf(tenantID int64) string { return fmt.Sprintf("gowms:demo:active:%d", tenantID) }

// Service 协调多租户演示会话锁、演示场景和按租户的演示数据重置。
type Service struct {
	cfg       *config.Config
	db        *gorm.DB
	rdb       *redis.Client
	inbound   *inboundservice.Service
	outbound  *outboundservice.Service
	stocktake *stocktakeservice.Service
	runMu     sync.Mutex
}

func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client,
	inbound *inboundservice.Service, outbound *outboundservice.Service, stocktake *stocktakeservice.Service) *Service {
	return &Service{cfg: cfg, db: db, rdb: rdb, inbound: inbound, outbound: outbound, stocktake: stocktake}
}

func (s *Service) Enabled() bool { return s.cfg != nil && s.cfg.Demo.Enabled && s.rdb != nil }

// IsDemoTenant 根据已验证 JWT 的租户判断，其他租户的同名用户不是演示账号。
func (s *Service) IsDemoTenant(ctx context.Context) bool {
	return s.cfg != nil && s.cfg.Demo.AccountIndex(tenant.FromContext(ctx)) > 0
}

// Username 返回当前请求租户对应的演示账号名（业务单据的操作人）。
// 兼容历史数据：非演示租户（如平台管理员触发）返回旧账号名 "demo"。
func (s *Service) Username(ctx context.Context) string {
	if s.cfg != nil {
		if i := s.cfg.Demo.AccountIndex(tenant.FromContext(ctx)); i > 0 {
			return s.cfg.Demo.AccountUsername(i)
		}
	}
	return "demo"
}

// demoTenantID 只允许配置中的演示租户，正数租户 ID 本身不代表可重置的演示空间。
func (s *Service) demoTenantID(ctx context.Context) (int64, error) {
	tid := tenant.FromContext(ctx)
	if !s.IsDemoTenant(ctx) {
		return 0, errcode.Forbidden
	}
	return tid, nil
}

func (s *Service) sessionTTL() time.Duration {
	if s.cfg == nil || s.cfg.Demo.SessionTTLSeconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(s.cfg.Demo.SessionTTLSeconds) * time.Second
}

func (s *Service) ValidateSession(ctx context.Context, sessionID string) error {
	if !s.Enabled() {
		return errcode.DemoDisabled
	}
	if sessionID == "" {
		return errcode.DemoSessionInvalid
	}
	tid := tenant.FromContext(ctx)
	if tid <= 0 {
		return errcode.DemoSessionInvalid
	}
	current, err := s.rdb.Get(ctx, activeSessionKeyOf(tid)).Result()
	if errors.Is(err, redis.Nil) {
		return errcode.DemoSessionInvalid
	}
	if err != nil {
		return err
	}
	if current != sessionID {
		return errcode.DemoSessionInvalid
	}
	return nil
}
