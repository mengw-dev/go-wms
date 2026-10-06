package idempotency

// 幂等记录的保留与定期清理：记录只用于重放窗口期内的重试，长期留存会无谓占用表空间。

import (
	"context"
	"time"

	"gorm.io/gorm"

	"gowms/internal/pkg/log"
)

const (
	// CleanupInterval 清理循环的执行间隔。
	CleanupInterval = 24 * time.Hour
	// RecordRetention 幂等记录保留时长：创建时间早于该时长的记录会被删除。
	RecordRetention = 7 * 24 * time.Hour
)

// Purge 物理删除 created_at 早于 before 的幂等记录，返回删除行数。
// Record 没有软删除字段，Delete 即真实删除。
func Purge(db *gorm.DB, before time.Time) (int64, error) {
	result := db.Where("created_at < ?", before).Delete(&Record{})
	return result.RowsAffected, result.Error
}

// RunCleanup 单实例循环清理：启动先执行一次，之后按 interval 定时执行，
// ctx 取消后退出（随进程重启自然恢复，无需数据库队列）。
func RunCleanup(ctx context.Context, db *gorm.DB, interval, retention time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		rows, err := Purge(db.WithContext(ctx), time.Now().Add(-retention))
		if err != nil {
			if ctx.Err() == nil {
				log.L().Error("purge idempotency records failed", "err", err)
			}
		} else if rows > 0 {
			log.L().Info("purged expired idempotency records", "rows", rows)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
