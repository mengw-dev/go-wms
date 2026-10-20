package testutil

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisFixturesIsolateCommandsScriptsAndCleanup(t *testing.T) {
	a, b := OpenTestRedis(t), OpenTestRedis(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := "gowms:demo:active:10098"
	if err := b.Set(ctx, key, "other-fixture", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	raw := redis.NewClient(b.Options())
	t.Cleanup(func() { _ = raw.Close() })
	var ownedPrefix string
	t.Run("owned namespace", func(t *testing.T) {
		owned, prefix := openTestRedis(t)
		ownedPrefix = prefix
		for i := 0; i < 270; i++ {
			if err := owned.Set(ctx, "cleanup:"+strings.Repeat("x", i), "owned", 0).Err(); err != nil {
				t.Fatal(err)
			}
		}
		if err := owned.Set(ctx, key, "owned", 0).Err(); err != nil {
			t.Fatal(err)
		}
	})
	keys := raw.Scan(ctx, 0, ownedPrefix+"*", 128).Iterator()
	if keys.Next(ctx) || keys.Err() != nil {
		t.Fatalf("fixture cleanup left keys or failed: %v", keys.Err())
	}
	if value, err := b.Get(ctx, key).Result(); err != nil || value != "other-fixture" {
		t.Fatalf("cleanup crossed namespaces: value=%q err=%v", value, err)
	}
	script := redis.NewScript(`return redis.call('INCR', KEYS[1])`)
	for i := int64(1); i <= 2; i++ {
		if got, err := script.Run(ctx, a, []string{key}).Int64(); err != nil || got != i {
			t.Fatalf("EVAL/EVALSHA isolation: got=%d err=%v", got, err)
		}
	}
	if _, err := a.Pipelined(ctx, func(p redis.Pipeliner) error {
		p.Set(ctx, "pipeline", "a", time.Minute)
		p.Del(ctx, key)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Incr(ctx, "pipeline-counter")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"pipeline": "a", "pipeline-counter": "1"} {
		if got, err := a.Get(ctx, key).Result(); err != nil || got != want {
			t.Fatalf("pipeline did not execute in its own namespace: key=%q got=%q err=%v", key, got, err)
		}
	}
	if n, err := a.Exists(ctx, key).Result(); err != nil || n != 0 {
		t.Fatalf("pipeline DEL did not execute: n=%d err=%v", n, err)
	}
	if value, err := b.Get(ctx, key).Result(); err != nil || value != "other-fixture" {
		t.Fatalf("script/DEL crossed namespaces: value=%q err=%v", value, err)
	}
	if n, err := b.Exists(ctx, "pipeline", "pipeline-counter").Result(); err != nil || n != 0 {
		t.Fatalf("pipeline crossed namespaces: n=%d err=%v", n, err)
	}
	for _, args := range [][]any{{"FLUSHDB"}, {"FLUSHALL"}, {"SELECT", 1}, {"EVAL", "return 1", 0}, {"SCRIPT", "FLUSH"}} {
		if err := a.Do(ctx, args...).Err(); err == nil || !strings.Contains(err.Error(), "test Redis:") {
			t.Fatalf("unsafe command %v was not rejected: %v", args, err)
		}
	}
}
