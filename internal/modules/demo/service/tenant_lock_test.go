package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func reserveDemoSession(t *testing.T, rdb *redis.Client, tid int64) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ctx = tenant.WithTenant(ctx, tid)
	sessionKey := activeSessionKeyOf(tid)
	lockKey := demoResetLockKeyOf(tid)
	if err := rdb.Del(ctx, sessionKey, lockKey).Err(); err != nil {
		t.Fatalf("cleanup demo keys: %v", err)
	}
	sessionID := uuid.NewString()
	if ok, err := rdb.SetNX(ctx, sessionKey, sessionID, time.Minute).Result(); err != nil || !ok {
		t.Fatalf("reserve demo session: ok=%v err=%v", ok, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		if err := rdb.Del(cleanupCtx, sessionKey, lockKey).Err(); err != nil {
			t.Error(err)
		}
	})
	return sessionID
}

func TestScenarioExecutionLockIsScopedPerTenant(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	s := &Service{cfg: &config.Config{Demo: config.DemoConfig{Enabled: true, Instances: 99, SessionTTLSeconds: 300}}, rdb: rdb}
	tid := int64(10098)
	sessionID := reserveDemoSession(t, rdb, tid)
	ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), tid), 10*time.Second)
	defer cancel()

	entered := make(chan struct{})
	releaseRun := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		_, finish, err := s.beginTenantRun(ctx, sessionID)
		if err != nil {
			firstDone <- err
			return
		}
		close(entered)
		<-releaseRun
		finish()
		firstDone <- nil
	}()

	select {
	case <-entered:
	case err := <-firstDone:
		t.Fatalf("first run failed: %v", err)
	case <-ctx.Done():
		t.Fatal("first run did not enter")
	}

	_, finishSecond, err := s.beginTenantRun(ctx, sessionID)
	if !errors.Is(err, errcode.DemoBusy) {
		if finishSecond != nil {
			finishSecond()
		}
		t.Fatalf("same tenant entered twice: %v", err)
	}

	close(releaseRun)
	if err := <-firstDone; err != nil {
		t.Fatalf("first run finish: %v", err)
	}

	_, finishAgain, err := s.beginTenantRun(ctx, sessionID)
	if err != nil {
		t.Fatalf("same tenant could not run after release: %v", err)
	}
	finishAgain()
}

func TestDifferentDemoTenantsRunConcurrently(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	s := &Service{cfg: &config.Config{Demo: config.DemoConfig{Enabled: true, Instances: 99, SessionTTLSeconds: 300}}, rdb: rdb}
	tidA, tidB := int64(10097), int64(10096)
	sessionA := reserveDemoSession(t, rdb, tidA)
	sessionB := reserveDemoSession(t, rdb, tidB)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	entered := make(chan struct{}, 2)
	releaseRuns := make(chan struct{})
	done := make(chan error, 2)
	run := func(tid int64, sessionID string) {
		runCtx := tenant.WithTenant(ctx, tid)
		_, finish, err := s.beginTenantRun(runCtx, sessionID)
		if err != nil {
			done <- err
			return
		}
		entered <- struct{}{}
		<-releaseRuns
		finish()
		done <- nil
	}

	go run(tidA, sessionA)
	go run(tidB, sessionB)
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case err := <-done:
			t.Fatalf("tenant run failed before both entered: %v", err)
		case <-ctx.Done():
			t.Fatal("different tenants blocked each other")
		}
	}
	close(releaseRuns)
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("tenant run failed: %v", err)
		}
	}
}

func TestOldSessionCannotResetNewSession(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	tid := int64(10095)
	oldToken := reserveDemoSession(t, rdb, tid)
	newToken := uuid.NewString()
	ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), tid), 5*time.Second)
	defer cancel()
	key := activeSessionKeyOf(tid)
	if err := rdb.Set(ctx, key, newToken, time.Minute).Err(); err != nil {
		t.Fatal(err)
	}

	s := &Service{cfg: &config.Config{Demo: config.DemoConfig{Enabled: true, Instances: 99, SessionTTLSeconds: 300}}, rdb: rdb}
	if err := s.resetLocked(ctx, oldToken); !errors.Is(err, errcode.DemoSessionInvalid) {
		t.Fatalf("old session reset result: %v", err)
	}
	if err := s.ReleaseSession(ctx, oldToken); !errors.Is(err, errcode.DemoSessionInvalid) {
		t.Fatalf("old session release result: %v", err)
	}
	if got, err := rdb.Get(ctx, key).Result(); err != nil || got != newToken {
		t.Fatalf("new session changed: got=%q err=%v", got, err)
	}
}
