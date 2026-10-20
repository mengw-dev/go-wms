package testutil

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// Only commands used by integration tests are allowed. Lua receives prefixed
// KEYS, so locks/session scripts exercise the real server without sharing keys.
type redisNamespaceHook struct{ prefix string }

func (h redisNamespaceHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h redisNamespaceHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := h.rewrite(cmd); err != nil {
			return err
		}
		return next(ctx, cmd)
	}
}

func (h redisNamespaceHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if err := h.rewrite(cmd); err != nil {
				return err
			}
		}
		return next(ctx, cmds)
	}
}

func (h redisNamespaceHook) rewrite(cmd redis.Cmder) error {
	args := cmd.Args()
	first, count := 1, 1
	switch cmd.Name() {
	case "ping", "hello", "auth", "multi", "exec":
		return nil
	case "client", "script":
		if len(args) > 1 {
			sub := strings.ToLower(fmt.Sprint(args[1]))
			if cmd.Name() == "client" && (sub == "setinfo" || sub == "maint_notifications") || cmd.Name() == "script" && (sub == "load" || sub == "exists") {
				return nil
			}
		}
		return fmt.Errorf("test Redis: unsupported %s subcommand", cmd.Name())
	case "get", "set", "setnx", "incr", "expire", "pexpire", "ttl", "pttl", "persist":
	case "del", "exists":
		count = len(args) - 1
	case "eval", "evalsha":
		if len(args) < 3 {
			return fmt.Errorf("test Redis: malformed %s", cmd.Name())
		}
		var err error
		count, err = strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil || count < 1 {
			return fmt.Errorf("test Redis: Lua must declare its KEYS")
		}
		first = 3
	default:
		return fmt.Errorf("test Redis: command %q has no namespace rule", cmd.Name())
	}
	if count < 1 || count > len(args)-first {
		return fmt.Errorf("test Redis: missing keys for %s", cmd.Name())
	}
	for i := first; i < first+count; i++ {
		key, ok := args[i].(string)
		if !ok {
			return fmt.Errorf("test Redis: key must be a string")
		}
		args[i] = h.prefix + key
	}
	return nil
}

func deleteRedisNamespace(ctx context.Context, rdb *redis.Client, prefix string) error {
	// This client has no hook. Scan/delete only this fixture's UUID namespace;
	// never FLUSHDB, FLUSHALL or delete unprefixed application keys.
	if !strings.HasPrefix(prefix, "gowms:test:") || len(prefix) < 40 {
		return fmt.Errorf("invalid test Redis namespace")
	}
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, prefix+"*", 128).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := rdb.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			return nil
		}
	}
}
