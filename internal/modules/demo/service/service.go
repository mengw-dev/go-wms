package service

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	inboundservice "gowms/internal/modules/inbound/service"
	outboundservice "gowms/internal/modules/outbound/service"
	stocktakeservice "gowms/internal/modules/stocktake/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
)

const (
	activeSessionKey = "gowms:demo:active"
	dirtyDataKey     = "gowms:demo:dirty"
)

// Service 协调单会话演示锁、演示场景和演示数据重置。
type Service struct {
	cfg       *config.Config
	db        *gorm.DB
	rdb       *redis.Client
	inbound   *inboundservice.Service
	outbound  *outboundservice.Service
	stocktake *stocktakeservice.Service
	resetMu   sync.Mutex
	runMu     sync.Mutex
}

func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client,
	inbound *inboundservice.Service, outbound *outboundservice.Service, stocktake *stocktakeservice.Service) *Service {
	return &Service{cfg: cfg, db: db, rdb: rdb, inbound: inbound, outbound: outbound, stocktake: stocktake}
}

func (s *Service) Enabled() bool { return s.cfg != nil && s.cfg.Demo.Enabled && s.rdb != nil }

func (s *Service) Username() string {
	if s.cfg == nil {
		return "demo"
	}
	return s.cfg.Demo.Username
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
	current, err := s.rdb.Get(ctx, activeSessionKey).Result()
	if err == redis.Nil || current != sessionID {
		return errcode.DemoSessionInvalid
	}
	if err != nil {
		return err
	}
	return nil
}
