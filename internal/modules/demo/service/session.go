package service

import (
	"context"
	"time"

	"github.com/google/uuid"

	"gowms/internal/bootstrap"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/lock"
)

const demoResetLockKey = "gowms:demo:reset-lock"

// SessionInfo 当前演示会话信息。
type SessionInfo struct {
	SessionID string `json:"session_id"`
	ExpiresIn int    `json:"expires_in"`
}

// AcquireSession 获取单实例演示会话。已有会话时返回忙碌错误。
func (s *Service) AcquireSession(ctx context.Context) (*SessionInfo, error) {
	if !s.Enabled() {
		return nil, errcode.DemoDisabled
	}
	sessionID := uuid.NewString()
	ttl := s.sessionTTL()
	ok, err := s.rdb.SetNX(ctx, activeSessionKey, sessionID, ttl).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		// 会话 TTL 会随对方操作动态续期，给不出准确的等待时间，统一提示稍后重试。
		return nil, errcode.DemoBusy
	}

	dirty, err := s.rdb.Exists(ctx, dirtyDataKey).Result()
	if err != nil {
		_ = s.rdb.Del(ctx, activeSessionKey).Err()
		return nil, err
	}
	if dirty > 0 {
		if err := s.resetLocked(ctx); err != nil {
			_ = s.rdb.Del(ctx, activeSessionKey).Err()
			return nil, err
		}
	}
	if err := s.rdb.Set(ctx, dirtyDataKey, "1", 0).Err(); err != nil {
		_ = s.rdb.Del(ctx, activeSessionKey).Err()
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
	if err := s.rdb.Expire(ctx, activeSessionKey, ttl).Err(); err != nil {
		return nil, err
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, nil
}

// ReleaseSession 释放演示会话并立即恢复演示数据。
func (s *Service) ReleaseSession(ctx context.Context, sessionID string) error {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.resetLocked(ctx); err != nil {
		return err
	}
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, activeSessionKey)
	pipe.Del(ctx, dirtyDataKey)
	_, err := pipe.Exec(ctx)
	return err
}

// Reset 重置当前演示会话的数据，但保留会话锁。
func (s *Service) Reset(ctx context.Context, sessionID string) error {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.resetLocked(ctx); err != nil {
		return err
	}
	return s.rdb.Set(ctx, dirtyDataKey, "1", 0).Err()
}

// SessionStatus 返回当前会话剩余时间。
func (s *Service) SessionStatus(ctx context.Context, sessionID string) (*SessionInfo, bool) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, false
	}
	ttl, err := s.rdb.TTL(ctx, activeSessionKey).Result()
	if err != nil || ttl < 0 {
		return nil, false
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, true
}

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
	return bootstrap.ResetDemoData(s.db)
}
