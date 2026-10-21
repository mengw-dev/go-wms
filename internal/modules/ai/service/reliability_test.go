package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"gowms/internal/modules/ai/client"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func TestAIQuotaConcurrentAndTTLRepair(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	s := New(config.AIConfig{RateLimitPerMin: 7, DailyLimitPerTenant: 3}, nil, rdb)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			if s.allowRate(context.Background(), 41) {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 7 {
		t.Fatalf("allowed %d requests, want 7", allowed.Load())
	}
	ctx := context.Background()
	key := "wms:ai:rl:41"
	assertQuotaTTL(t, rdb, key, time.Minute)
	before := rdb.PTTL(ctx, key).Val()
	if err := rdb.PExpire(ctx, key, 10*time.Second).Err(); err != nil {
		t.Fatal(err)
	}
	s.allowRate(ctx, 41)
	if after := rdb.PTTL(ctx, key).Val(); after <= 0 || after > 10*time.Second || after >= before {
		t.Fatalf("later request extended window: %v", after)
	}
	if err := rdb.Set(ctx, key, "32", 0).Err(); err != nil {
		t.Fatal(err)
	}
	s.allowRate(ctx, 41)
	assertQuotaTTL(t, rdb, key, time.Minute)

	tenantCtx := tenant.WithExactTenant(ctx, 51)
	for i := range 4 {
		if got := s.allowDailyQuota(tenantCtx); got != (i < 3) {
			t.Fatalf("daily request %d allowed=%v", i, got)
		}
	}
	dailyKey := fmt.Sprintf("wms:ai:daily:51:%s", time.Now().Format("20060102"))
	assertQuotaTTL(t, rdb, dailyKey, 24*time.Hour)
	if err := rdb.Set(ctx, dailyKey, "4", 0).Err(); err != nil {
		t.Fatal(err)
	}
	s.allowDailyQuota(tenantCtx)
	assertQuotaTTL(t, rdb, dailyKey, 24*time.Hour)
	if !s.allowDailyQuota(tenant.WithExactTenant(ctx, 52)) || !s.allowRate(ctx, 42) {
		t.Fatal("another tenant/user inherited quota")
	}
}

func assertQuotaTTL(t *testing.T, rdb *redis.Client, key string, maxTTL time.Duration) {
	t.Helper()
	ttl, err := rdb.PTTL(context.Background(), key).Result()
	if err != nil || ttl <= 0 || ttl > maxTTL {
		t.Fatalf("counter %s has invalid TTL %v: %v", key, ttl, err)
	}
}

// Losing the script's reply cannot split its committed INCR and expiry.
type lostQuotaReply struct{ fail atomic.Bool }

func (h *lostQuotaReply) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h *lostQuotaReply) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *lostQuotaReply) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		if err == nil && (cmd.Name() == "eval" || cmd.Name() == "evalsha") && h.fail.CompareAndSwap(true, false) {
			return errors.New("injected lost script reply")
		}
		return err
	}
}

func TestAIQuotaLostReplyStillExpires(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	hook := &lostQuotaReply{}
	hook.fail.Store(true)
	rdb.AddHook(hook)
	s := New(config.AIConfig{RateLimitPerMin: 1}, nil, rdb)
	if !s.allowRate(context.Background(), 71) {
		t.Fatal("Redis errors must retain documented fail-open policy")
	}
	assertQuotaTTL(t, rdb, "wms:ai:rl:71", time.Minute)
	if s.allowRate(context.Background(), 71) {
		t.Fatal("lost reply must not roll back a committed counter")
	}
}

func TestAIQuotaRepairsInterruptedLegacyIncrement(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	ctx, cancel := context.WithCancel(context.Background())
	key := "wms:ai:rl:81"
	if err := rdb.Incr(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	cancel() // Reproduce the old INCR -> interruption -> EXPIRE gap.
	if err := rdb.Expire(ctx, key, time.Minute).Err(); err == nil {
		t.Fatal("fault injection did not interrupt expiry")
	}
	if ttl := rdb.PTTL(context.Background(), key).Val(); ttl != -1 {
		t.Fatalf("expected legacy counter without TTL, got %v", ttl)
	}
	s := New(config.AIConfig{RateLimitPerMin: 3}, nil, rdb)
	if !s.allowRate(context.Background(), 81) {
		t.Fatal("repaired counter should retain its existing count")
	}
	assertQuotaTTL(t, rdb, key, time.Minute)
}

func TestAIModelDeadlineAndFallback(t *testing.T) {
	for _, scenario := range []string{"fallback", "primary-timeout", "all-timeout", "cancel", "unauthorized", "forbidden", "same-model", "no-backup"} {
		t.Run(scenario, func(t *testing.T) {
			var primary, backup atomic.Int64
			started := make(chan struct{})
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Model string }
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				_, _ = io.Copy(io.Discard, r.Body)
				_ = r.Body.Close()
				if body.Model == "primary" {
					primary.Add(1)
					close(started)
					if scenario == "unauthorized" || scenario == "forbidden" {
						status := http.StatusUnauthorized
						if scenario == "forbidden" {
							status = http.StatusForbidden
						}
						w.WriteHeader(status)
						_, _ = w.Write([]byte(`{"error":{"code":"auth","message":"denied"}}`))
						return
					}
					if scenario != "fallback" && scenario != "same-model" && scenario != "no-backup" {
						<-r.Context().Done()
						return
					}
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"error":{"code":"busy","message":"busy"}}`))
					return
				}
				backup.Add(1)
				if scenario == "all-timeout" {
					<-r.Context().Done()
					return
				}
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"备用答案"}}]}`))
			}))
			defer provider.Close()
			cfg := config.AIConfig{APIKey: "test-only", Model: "primary", BackupModel: "backup", BaseURL: provider.URL}
			if scenario == "same-model" {
				cfg.BackupModel = cfg.Model
			}
			if scenario == "no-backup" {
				cfg.BackupModel = ""
			}
			s := New(cfg, nil, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
			defer cancel()
			if scenario == "cancel" {
				go func() {
					select {
					case <-started:
						cancel()
					case <-ctx.Done():
					}
				}()
			}
			begin := time.Now()
			answer, err := s.chatModels(ctx, []client.Message{{Role: "user", Content: "库存"}})
			if scenario == "fallback" || scenario == "primary-timeout" {
				if err != nil || answer != "备用答案" || backup.Load() != 1 {
					t.Fatalf("answer=%q err=%v backup=%d", answer, err, backup.Load())
				}
			} else {
				if !errors.Is(err, errcode.AIServiceUnavailable) {
					t.Fatalf("expected unavailable, got %v", err)
				}
				if scenario != "all-timeout" && backup.Load() != 0 {
					t.Fatal("fallback called after cancellation/auth failure or without a distinct model")
				}
			}
			if primary.Load() != 1 || time.Since(begin) > 2*time.Second {
				t.Fatal("unexpected retries or exceeded shared budget")
			}
		})
	}
}

func TestAIChatBudgetIncludesSnapshot(t *testing.T) {
	s, db := newTestService(t)
	s.cfg.APIKey = "test-only"
	s.cfg.RequestTimeoutSeconds = 1
	if err := db.Callback().Query().Before("gorm:query").Register("ai:slow-snapshot", func(tx *gorm.DB) {
		<-tx.Statement.Context.Done()
	}); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err := s.Chat(context.Background(), 1, "库存")
	if !errors.Is(err, errcode.AIServiceUnavailable) || time.Since(start) > 3*time.Second {
		t.Fatalf("snapshot escaped request budget: %v", err)
	}
}

func TestAIHTTPDisconnectStopsModel(t *testing.T) {
	_, db := newTestService(t)
	started, stopped := make(chan struct{}), make(chan struct{})
	var calls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		if calls.Add(1) == 1 {
			close(started)
		}
		<-r.Context().Done()
		if calls.Load() == 1 {
			close(stopped)
		}
	}))
	defer provider.Close()
	s := New(config.AIConfig{APIKey: "test-only", Model: "primary", BackupModel: "backup", BaseURL: provider.URL, RequestTimeoutSeconds: 5}, db, nil)
	ingress := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = s.Chat(r.Context(), 1, "库存")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ingress.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ingress.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		resp, requestErr := http.DefaultClient.Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
		done <- requestErr
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("model request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request not canceled: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("browser-side request did not end")
	}
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("provider request survived incoming HTTP cancellation")
	}
	if calls.Load() != 1 {
		t.Fatal("canceling the caller started a backup model")
	}
}

func TestAICanceledRequestDoesNotAccessDependencies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := New(config.AIConfig{APIKey: "test-only"}, nil, nil)
	if _, err := s.Chat(ctx, 1, "库存"); !errors.Is(err, errcode.AIServiceUnavailable) {
		t.Fatal(err)
	}
}
