package orderno

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Generator 单号生成器。
//
// 主方案：Redis Lua 原子执行 INCR + TTL 兜底（按天重置，TTL 48h）。
// 降级方案：日期 + F + UUID，不依赖进程计数、节点号或跨日重置。
// 兜底：调用方依赖数据库唯一索引，插入冲突时重新生成（重试逻辑在各业务 Service）。
type Generator struct {
	rdb redis.UniversalClient
}

// New 允许 rdb 为 nil（完全降级模式）。
func New(rdb redis.UniversalClient) *Generator {
	return &Generator{rdb: rdb}
}

// ttlSeconds 48 小时，覆盖跨天后仍能命中昨日 key 的场景。
const ttlSeconds = 48 * 3600

// 日期已包含在 key 中；原子自增并补设 TTL，避免故障留下永久占用内存的旧日期 key。
var luaScript = redis.NewScript(`
local v = redis.call('INCR', KEYS[1])
if redis.call('TTL', KEYS[1]) < 0 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return v
`)

// Next 正常生成 {prefix}{yyyyMMdd}{至少6位序号}；降级使用 F 标记与 UUID。
func (g *Generator) Next(ctx context.Context, prefix string) string {
	day := time.Now().Format("20060102")
	if g.rdb != nil {
		redisCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer cancel()
		key := fmt.Sprintf("gowms:orderno:%s:%s", prefix, day)
		seq, err := luaScript.Run(redisCtx, g.rdb, []string{key}, ttlSeconds).Int64()
		if err == nil {
			return fmt.Sprintf("%s%s%06d", prefix, day, seq)
		}
		// Redis 故障 → 降级
	}
	return prefix + day + "F" + strings.ReplaceAll(uuid.NewString(), "-", "")
}
