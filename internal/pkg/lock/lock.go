package lock

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"gowms/internal/pkg/log"
)

// Locker Redis 分布式锁：SET NX EX 加锁 + Lua 校验持有者后释放（防止误删他人的锁）。
type Locker struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Locker { return &Locker{rdb: rdb} }

var unlockScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) == ARGV[1] then
  return redis.call('DEL', KEYS[1])
end
return 0
`)

// Lock 获取锁；成功返回 release 函数，失败返回 ok=false。
func (l *Locker) Lock(ctx context.Context, key string, ttl time.Duration) (release func(), ok bool, err error) {
	token := uuid.NewString()
	ok, err = l.rdb.SetNX(ctx, key, token, ttl).Result()
	if err != nil || !ok {
		return nil, false, err
	}
	release = func() {
		// 请求可能已经取消，但释放锁仍需尝试；独立超时限制退出等待。
		releaseCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := unlockScript.Run(releaseCtx, l.rdb, []string{key}, token).Err(); err != nil {
			log.L().Warn("failed to release distributed lock", "key", key, "err", err)
		}
	}
	return release, true, nil
}
