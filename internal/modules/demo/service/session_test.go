package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func TestSessionOperationsRejectPreviousOwner(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// 只创建当前测试独有的 key；不使用真实演示租户，不清空 Redis 数据库。
	tid := time.Now().UnixNano()
	ctx = tenant.WithTenant(ctx, tid)
	key := activeSessionKeyOf(tid)
	oldToken, newToken := uuid.NewString(), uuid.NewString()
	if ok, err := rdb.SetNX(ctx, key, oldToken, time.Minute).Result(); err != nil || !ok {
		t.Fatalf("reserve unique test key: %v %v", ok, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := rdb.Del(cleanupCtx, key, demoResetLockKeyOf(tid)).Err(); err != nil {
			t.Error(err)
		}
	})
	s := &Service{cfg: &config.Config{Demo: config.DemoConfig{Enabled: true, SessionTTLSeconds: 300}}, rdb: rdb}
	if err := s.ValidateSession(ctx, oldToken); err != nil {
		t.Fatal(err)
	}
	// 模拟校验后会话过期并被另一访客领取。
	if err := rdb.Set(ctx, key, newToken, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Heartbeat(ctx, oldToken); !errors.Is(err, errcode.DemoSessionInvalid) {
		t.Fatalf("old heartbeat: %v", err)
	}
	if ttl, err := rdb.TTL(ctx, key).Result(); err != nil || ttl > time.Minute {
		t.Fatalf("old token renewed current session: ttl=%v err=%v", ttl, err)
	}
	if err := s.deleteSession(ctx, oldToken); !errors.Is(err, errcode.DemoSessionInvalid) {
		t.Fatalf("old delete: %v", err)
	}
	if _, ok := s.SessionStatus(ctx, oldToken); ok {
		t.Fatal("old token read new session status")
	}
	// 测试独有的随机租户不属于配置的演示空间，不能进入重置（此处 db=nil）。
	if err := s.resetLocked(ctx, oldToken); !errors.Is(err, errcode.Forbidden) {
		t.Fatalf("non-demo reset: %v", err)
	}
	if got, err := rdb.Get(ctx, key).Result(); err != nil || got != newToken {
		t.Fatalf("current session overwritten: %q %v", got, err)
	}
	if _, err := s.Heartbeat(ctx, newToken); err != nil {
		t.Fatal(err)
	}
	if info, ok := s.SessionStatus(ctx, newToken); !ok || info.SessionID != newToken || info.ExpiresIn < 290 {
		t.Fatalf("current session: %+v ok=%v", info, ok)
	}
	if err := s.deleteSession(ctx, newToken); err != nil {
		t.Fatal(err)
	}
	if n, err := rdb.Exists(ctx, key).Result(); err != nil || n != 0 {
		t.Fatalf("session was not released: %d %v", n, err)
	}
}

func TestExecutionLockPreventsSessionReplacementDuringLongRun(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tid := 10000 + (time.Now().UnixNano() % 99) + 1
	ctx = tenant.WithTenant(ctx, tid)
	sessionID := uuid.NewString()
	sessionKey := activeSessionKeyOf(tid)
	lockKey := demoResetLockKeyOf(tid)
	if ok, err := rdb.SetNX(ctx, sessionKey, sessionID, time.Minute).Result(); err != nil || !ok {
		t.Fatalf("reserve test session: %v %v", ok, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if err := rdb.Del(cleanupCtx, sessionKey, lockKey).Err(); err != nil {
			t.Error(err)
		}
	})

	s := &Service{cfg: &config.Config{Demo: config.DemoConfig{Enabled: true, Instances: 99, SessionTTLSeconds: 1}}, rdb: rdb}
	started := make(chan struct{})
	releaseRun := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, finish, err := s.beginTenantRun(ctx, sessionID)
		if err != nil {
			done <- err
			return
		}
		close(started)
		<-releaseRun
		finish()
		done <- nil
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("long run did not acquire execution lock")
	}
	// 模拟会话租约先于长请求过期；执行锁仍必须阻止新访客领取同一租户。
	if err := rdb.Del(ctx, sessionKey).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcquireSession(ctx); !errors.Is(err, errcode.DemoBusy) {
		t.Fatalf("session was replaced while long run held execution lock: %v", err)
	}
	close(releaseRun)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("long run did not release execution lock")
	}
}
