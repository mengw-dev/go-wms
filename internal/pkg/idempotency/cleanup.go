package idempotency

// 幂等记录的保留与定期清理：记录只用于重放窗口期内的重试，长期留存会无谓占用表空间。

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"gowms/internal/pkg/log"
	"gowms/internal/pkg/observability"
)

const (
	// CleanupInterval 清理循环的执行间隔。
	CleanupInterval = 24 * time.Hour
	// RecordRetention 幂等记录保留时长：创建时间早于该时长的记录会被删除。
	RecordRetention = 7 * 24 * time.Hour
	// CleanupBatchSize limits the number of primary keys selected for each delete.
	CleanupBatchSize       = 500
	CleanupMaxBatches      = 20
	CleanupBatchTimeout    = 2 * time.Second
	CleanupRunTimeout      = 30 * time.Second
	CleanupBacklogInterval = time.Minute
	cleanupBatchPause      = 20 * time.Millisecond
)

// Purge 分批物理删除 created_at 早于 before 的幂等记录，返回已确认删除的行数。
// 使用非事务连接，每批独立提交；最多清理 CleanupMaxBatches 批，后续调用继续清理。
// 错误或取消不回滚前面已提交的批次；调用方 context 的更早截止时间仍有效。
func Purge(db *gorm.DB, before time.Time) (int64, error) {
	result, err := purge(db, before)
	return result.rows, err
}

type purgeResult struct {
	rows    int64
	batches int
	limited bool
}

func purge(db *gorm.DB, before time.Time) (result purgeResult, err error) {
	ctx, cancel := context.WithTimeout(db.Statement.Context, CleanupRunTimeout)
	defer cancel()
	for range CleanupMaxBatches {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		batchCtx, stop := context.WithTimeout(ctx, CleanupBatchTimeout)
		var ids []int64
		err := db.WithContext(batchCtx).Model(&Record{}).Where("created_at < ?", before).
			Order("created_at, id").Limit(CleanupBatchSize).Pluck("id", &ids).Error
		if err != nil {
			if batchCtx.Err() != nil {
				err = batchCtx.Err()
			}
			stop()
			return result, err
		}
		if len(ids) == 0 {
			stop()
			return result, nil
		}
		// Recheck expiry at deletion time. Both queries retain GORM tenant scope;
		// concurrent cleaners may delete the same selected keys first, yielding zero.
		deleted := db.WithContext(batchCtx).Where("id IN ? AND created_at < ?", ids, before).Delete(&Record{})
		err = deleted.Error
		if err != nil && batchCtx.Err() != nil {
			err = batchCtx.Err()
		}
		stop()
		if err != nil {
			return result, err
		}
		result.rows += deleted.RowsAffected
		result.batches++
		if len(ids) < CleanupBatchSize {
			return result, nil
		}
		if result.batches < CleanupMaxBatches {
			timer := time.NewTimer(cleanupBatchPause)
			select {
			case <-ctx.Done():
				timer.Stop()
				return result, ctx.Err()
			case <-timer.C:
			}
		}
	}
	result.limited = true // A full final selection may leave more work; do not claim an exact backlog.
	return result, nil
}

// RunCleanup 启动先执行一轮；达到批数上限或失败时提前重试，ctx取消后退出。
// 调用方等待本函数退出后才能关闭数据库；metrics 可为 nil。
func RunCleanup(ctx context.Context, db *gorm.DB, interval, retention time.Duration, metrics *observability.Metrics) {
	if interval <= 0 || retention <= 0 {
		log.L().Error("invalid idempotency cleanup interval or retention")
		return
	}
	for ctx.Err() == nil {
		start := time.Now()
		result, err := purge(db.WithContext(ctx), start.Add(-retention))
		outcome := "completed"
		if result.limited {
			outcome = "limited"
		}
		if err != nil {
			outcome = "error"
			if errors.Is(err, context.DeadlineExceeded) {
				outcome = "timeout"
			}
			if ctx.Err() != nil {
				outcome = "canceled"
			}
		}
		metrics.RecordIdempotencyCleanup(outcome, result.rows, result.batches, time.Since(start))
		if err != nil {
			if ctx.Err() == nil {
				log.L().Error("purge idempotency records failed", "rows", result.rows, "batches", result.batches, "err", err)
			}
		} else if result.rows > 0 || result.limited {
			log.L().Info("purged expired idempotency records", "rows", result.rows, "batches", result.batches, "limited", result.limited)
		}
		timer := time.NewTimer(nextCleanupDelay(interval, result.limited, err))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func nextCleanupDelay(interval time.Duration, limited bool, err error) time.Duration {
	if limited || err != nil {
		return min(interval, CleanupBacklogInterval)
	}
	return interval
}
