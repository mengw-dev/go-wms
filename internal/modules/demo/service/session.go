package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"gowms/internal/bootstrap"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/lock"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/tenant"
)

const demoExecutionTTL = 10 * time.Minute

func demoResetLockKeyOf(tenantID int64) string {
	return fmt.Sprintf("gowms:demo:reset-lock:%d", tenantID)
}

var renewSessionScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return 0
`)

var deleteSessionScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

var sessionTTLScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('PTTL', KEYS[1])
end
return -1
`)

// SessionInfo 当前演示会话信息。
type SessionInfo struct {
	SessionID string `json:"session_id"`
	ExpiresIn int    `json:"expires_in"`
}

// DemoAccountInfo 空闲演示账号信息（登录页"在线体验"领取）。
type DemoAccountInfo struct {
	TenantID int64  `json:"tenant_id,string"`
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
			TenantID: s.cfg.Demo.AccountTenantID(i),
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
	if err := s.resetLocked(ctx, sessionID); err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if cleanupErr := s.deleteSession(cleanupCtx, sessionID); cleanupErr != nil {
			log.WithContext(ctx).Warn("cleanup demo session failed", "err", cleanupErr)
		}
		return nil, err
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, nil
}

// Heartbeat 续期演示会话。
func (s *Service) Heartbeat(ctx context.Context, sessionID string) (*SessionInfo, error) {
	ttl := s.sessionTTL()
	if err := s.renewSession(ctx, sessionID, ttl); err != nil {
		return nil, err
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl.Seconds())}, nil
}

// ReleaseSession 释放演示会话并立即恢复该租户的演示数据。
func (s *Service) ReleaseSession(ctx context.Context, sessionID string) error {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return err
	}
	if err := s.resetLocked(ctx, sessionID); err != nil {
		return err
	}
	return s.deleteSession(ctx, sessionID)
}

// Reset 重置当前演示会话租户的数据，但保留会话锁。
func (s *Service) Reset(ctx context.Context, sessionID string) error {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return err
	}
	return s.resetLocked(ctx, sessionID)
}

// SessionStatus 返回当前会话剩余时间。
func (s *Service) SessionStatus(ctx context.Context, sessionID string) (*SessionInfo, bool) {
	if !s.Enabled() || sessionID == "" || tenant.FromContext(ctx) <= 0 {
		return nil, false
	}
	ttl, err := sessionTTLScript.Run(ctx, s.rdb, []string{activeSessionKeyOf(tenant.FromContext(ctx))}, sessionID).Int64()
	if err != nil || ttl < 0 {
		return nil, false
	}
	return &SessionInfo{SessionID: sessionID, ExpiresIn: int(ttl / 1000)}, true
}

func (s *Service) renewSession(ctx context.Context, sessionID string, ttl time.Duration) error {
	if !s.Enabled() {
		return errcode.DemoDisabled
	}
	if sessionID == "" || tenant.FromContext(ctx) <= 0 {
		return errcode.DemoSessionInvalid
	}
	n, err := renewSessionScript.Run(ctx, s.rdb, []string{activeSessionKeyOf(tenant.FromContext(ctx))}, sessionID, ttl.Milliseconds()).Int64()
	if err != nil {
		return err
	}
	if n != 1 {
		return errcode.DemoSessionInvalid
	}
	return nil
}

func (s *Service) deleteSession(ctx context.Context, sessionID string) error {
	if !s.Enabled() {
		return errcode.DemoDisabled
	}
	if sessionID == "" || tenant.FromContext(ctx) <= 0 {
		return errcode.DemoSessionInvalid
	}
	n, err := deleteSessionScript.Run(ctx, s.rdb, []string{activeSessionKeyOf(tenant.FromContext(ctx))}, sessionID).Int64()
	if err != nil {
		return err
	}
	if n != 1 {
		return errcode.DemoSessionInvalid
	}
	return nil
}

// resetLocked 重置当前请求租户的演示数据。
// 按租户互斥；取得重置锁后再次核对会话，不能依赖等待之前的校验结果。
func (s *Service) resetLocked(ctx context.Context, sessionID string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tid, err := s.demoTenantID(ctx)
	if err != nil {
		return err
	}
	release, ok, err := lock.New(s.rdb).Lock(ctx, demoResetLockKeyOf(tid), 2*time.Minute)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.DemoBusy
	}
	defer release()

	// 重置期限小于锁和会话租约；数据库失败时事务回滚。进程长时间暂停仍需单独考虑租约失效。
	if err := s.renewSession(ctx, sessionID, max(s.sessionTTL(), 2*time.Minute)); err != nil {
		return err
	}
	return bootstrap.ResetDemoData(ctx, s.db, tid)
}

// beginTenantRun 为一个完整演示请求持有租户级 Redis 执行锁，并把会话
// 租约延长到执行上限。请求结束后再恢复普通会话 TTL，避免长任务期间
// 新访客领取同一演示租户并操作同一份数据。
func (s *Service) beginTenantRun(ctx context.Context, sessionID string) (context.Context, func(), error) {
	tid, err := s.demoTenantID(ctx)
	if err != nil {
		return nil, nil, err
	}
	runCtx, cancelRun := context.WithTimeout(ctx, demoExecutionTTL)
	release, ok, err := lock.New(s.rdb).Lock(runCtx, demoResetLockKeyOf(tid), demoExecutionTTL)
	if err != nil {
		cancelRun()
		return nil, nil, err
	}
	if !ok {
		cancelRun()
		return nil, nil, errcode.DemoBusy
	}
	if err := s.renewSession(runCtx, sessionID, max(s.sessionTTL(), demoExecutionTTL)); err != nil {
		release()
		cancelRun()
		return nil, nil, err
	}
	finish := func() {
		if ctx.Err() == nil {
			restoreCtx, cancelRestore := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			if err := s.renewSession(restoreCtx, sessionID, s.sessionTTL()); err != nil {
				log.WithContext(restoreCtx).Warn("restore demo session ttl failed", "session_id", sessionID, "err", err)
			}
			cancelRestore()
		}
		release()
		cancelRun()
	}
	return runCtx, finish, nil
}
