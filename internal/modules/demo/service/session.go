package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gowms/internal/bootstrap"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/lock"
	"gowms/internal/pkg/tenant"
)

const demoResetLockKey = "gowms:demo:reset-lock"

// SessionInfo 当前演示会话信息。
type SessionInfo struct {
	SessionID string `json:"session_id"`
	ExpiresIn int    `json:"expires_in"`
}

// DemoAccountInfo 空闲演示账号信息（登录页"在线体验"领取）。
type DemoAccountInfo struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Total    int    `json:"total"`    // 演示席位总数
	Occupied int    `json:"occupied"` // 已占用席位数（不含本次分配）
}

// ClaimAccount 返回第一个空闲的演示账号（按 Redis 会话锁判断占用）。
// 互斥最终以登录后的 AcquireSession 抢锁为准，这里只是分配建议；
// 极端并发下两个访客领到同一账号时，后抢锁者会收到 DemoBusy。
func (s *Service) ClaimAccount(ctx context.Context) (*DemoAccountInfo, error) {
	if !s.Enabled() {
		return nil, errcode.DemoDisabled
	}
	occupied := 0
	for i := 1; i <= s.cfg.Demo.Instances; i++ {
		exists, err := s.rdb.Exists(ctx, activeSessionKeyOf(s.cfg.Demo.AccountTenantID(i))).Result()
		if err != nil {
			return nil, err // Redis 故障时无法判断占用，直接报错（不降级到固定账号，避免撞车）
		}
		if exists > 0 {
			occupied++
			continue
		}
		return &DemoAccountInfo{
			Username: s.cfg.Demo.AccountUsername(i),
			Password: s.cfg.Demo.Password,
			Total:    s.cfg.Demo.Instances,
			Occupied: occupied,
		}, nil
	}
	return nil, errcode.DemoBusy
}

// AcquireSession 获取当前租户的演示会话（同一演示账号同时只有一个访客）。
// 每次进入都无条件重置该租户数据：首访自动种入初始数据，复访清掉上一手访客的改动。
func (s *Service) AcquireSession(ctx context.Context) (*SessionInfo, error) {
	if !s.Enabled() {
		return nil, errcode.DemoDisabled
	}
	tid, err := s.demoTenantID(ctx)
	if err != nil {
		return nil, err
	}
	sessionID := uuid.NewString()
	ttl := s.sessionTTL()
	ok, err := s.rdb.SetNX(ctx, activeSessionKeyOf(tid), sessionID, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		// 会话 TTL 会随对方操作动态续期，给不出准确的等待时间，统一提示稍后重试。
		return nil, errcode.DemoBusy
	}
	if err := s.resetLocked(ctx); err != nil {
		_ = s.rdb.Del(ctx, activeSessionKeyOf(tid)).Err()
		return nil, err
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, nil
}

// Heartbeat 续期演示会话。
func (s *Service) Heartbeat(ctx context.Context, sessionID string) (*SessionInfo, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	ttl := s.sessionTTL()
	if err := s.rdb.Expire(ctx, activeSessionKeyOf(tenant.FromContext(ctx)), ttl).Err(); err != nil {
		return nil, err
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, nil
}

// ReleaseSession 释放演示会话并立即恢复该租户的演示数据。
func (s *Service) ReleaseSession(ctx context.Context, sessionID string) error {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.resetLocked(ctx); err != nil {
		return err
	}
	return s.rdb.Del(ctx, activeSessionKeyOf(tenant.FromContext(ctx))).Err()
}

// Reset 重置当前演示会话租户的数据，但保留会话锁。
func (s *Service) Reset(ctx context.Context, sessionID string) error {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return err
	}
	return s.resetLocked(ctx)
}

// SessionStatus 返回当前会话剩余时间。
func (s *Service) SessionStatus(ctx context.Context, sessionID string) (*SessionInfo, bool) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, false
	}
	ttl, err := s.rdb.TTL(ctx, activeSessionKeyOf(tenant.FromContext(ctx))).Result()
	if err != nil || ttl < 0 {
		return nil, false
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, true
}

// resetLocked 重置当前请求租户的演示数据。
// 进程内 resetMu 与分布式锁均为全局串行：重置是毫秒级操作，跨租户排队等待无感知，
// 换取实现简单（避免按租户维护锁集合）。
func (s *Service) resetLocked(ctx context.Context) error {
	release, ok, err := lock.New(s.rdb).Lock(ctx, demoResetLockKey, 2*time.Minute)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.DemoBusy
	}
	defer release()

	s.resetMu.Lock()
	defer s.resetMu.Unlock()
	return bootstrap.ResetDemoData(ctx, s.db, tenant.FromContext(ctx))
}
