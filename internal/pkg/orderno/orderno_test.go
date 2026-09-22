package orderno

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"gowms/internal/testutil"
)

func TestFallbackAcrossGenerators(t *testing.T) {
	const workers, perWorker = 12, 100
	numbers := make(chan string, workers*perWorker)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			// 不同实例没有共享计数器，也不需要同时切换日期。
			g := New(nil)
			for range perWorker {
				numbers <- g.Next(context.Background(), "RK")
			}
		})
	}
	wg.Wait()
	close(numbers)
	seen := make(map[string]struct{}, workers*perWorker)
	for number := range numbers {
		if len(number) != 43 || number[10] != 'F' || !strings.HasPrefix(number, "RK") {
			t.Fatalf("unexpected fallback format: %s", number)
		}
		if _, err := uuid.Parse(number[11:]); err != nil {
			t.Fatalf("fallback UUID: %v", err)
		}
		if _, exists := seen[number]; exists {
			t.Fatalf("duplicate number: %s", number)
		}
		seen[number] = struct{}{}
	}
}

func TestRedisSequenceAndTTLRepair(t *testing.T) {
	rdb := testutil.OpenTestRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	prefix := "test-" + uuid.NewString()
	day := time.Now().Format("20060102")
	key := fmt.Sprintf("gowms:orderno:%s:%s", prefix, day)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := rdb.Del(cleanupCtx, key).Err(); err != nil {
			t.Error(err)
		}
	})
	g := New(rdb)
	for i := 1; i <= 3; i++ {
		want := fmt.Sprintf("%s%s%06d", prefix, day, i)
		if got := g.Next(ctx, prefix); got != want {
			t.Fatalf("number=%s want=%s", got, want)
		}
	}
	if err := rdb.Persist(ctx, key).Err(); err != nil {
		t.Fatal(err)
	}
	_ = g.Next(ctx, prefix)
	if ttl, err := rdb.TTL(ctx, key).Result(); err != nil || ttl <= 0 || ttl > 48*time.Hour {
		t.Fatalf("TTL not restored: %v %v", ttl, err)
	}
	canceled, stop := context.WithCancel(ctx)
	stop()
	if got := g.Next(canceled, "RK"); len(got) != 43 || got[10] != 'F' {
		t.Fatalf("Redis failure did not fall back: %s", got)
	}
}
