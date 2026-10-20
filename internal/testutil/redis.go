package testutil

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// OpenTestRedis returns an isolated Redis client for integration tests.
// When WMS_TEST_REQUIRED=1, missing configuration or an unavailable Redis
// server fails the test instead of silently skipping Redis coverage.
func OpenTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	rdb, _ := openTestRedis(t)
	return rdb
}

func openTestRedis(t *testing.T) (*redis.Client, string) {
	t.Helper()
	addr := strings.TrimSpace(os.Getenv("WMS_TEST_REDIS_ADDR"))
	required := os.Getenv("WMS_TEST_REQUIRED") == "1"
	if addr == "" {
		if required {
			t.Fatal("redis required but WMS_TEST_REDIS_ADDR is empty")
		}
		t.Skip("set WMS_TEST_REDIS_ADDR to run Redis integration tests")
	}
	rdb := redis.NewClient(&redis.Options{
		Addr:                  addr,
		Password:              os.Getenv("WMS_TEST_REDIS_PASSWORD"),
		ContextTimeoutEnabled: true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		if required {
			t.Fatalf("redis required but unavailable at %s: %v", addr, err)
		}
		t.Skipf("redis unavailable, skip: %v", err)
	}
	prefix := "gowms:test:" + uuid.NewString() + ":"
	rdb.AddHook(redisNamespaceHook{prefix: prefix})
	t.Cleanup(func() {
		cleanup := redis.NewClient(rdb.Options())
		defer func() { _ = cleanup.Close(); _ = rdb.Close() }()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := deleteRedisNamespace(ctx, cleanup, prefix); err != nil {
			t.Errorf("cleanup test Redis namespace: %v", err)
		}
	})
	return rdb, prefix
}
