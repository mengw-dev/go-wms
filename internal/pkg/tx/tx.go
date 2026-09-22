// Package tx 提供事务重试、冲突分类和事务错误处理。
package tx

import (
	"context"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
)

const (
	// MaxOrderNoRetry 单号唯一索引冲突时的最大尝试次数（包含首次执行）。
	MaxOrderNoRetry = 3
	// MaxTxRetry 事务并发冲突时的最大尝试次数（包含首次执行）。
	MaxTxRetry = 3
)

// Manager 提供业务事务边界；Repository 使用调用方传入的事务连接。
type Manager struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Manager { return &Manager{db: db} }

// DB 返回非事务连接，用于查询。
func (m *Manager) DB() *gorm.DB { return m.db }

// Tx 在事务内执行 fn，panic 或返回 error 时回滚。
func (m *Manager) Tx(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return m.db.WithContext(ctx).Transaction(fn)
}

// retryBackoff 冲突重试基础退避，第 i 次重试等待 (i+1)*retryBackoff（50ms、100ms、150ms...）。
const retryBackoff = 50 * time.Millisecond

// TxRetry 在事务内执行 fn；遇到并发冲突（乐观锁失败、MySQL 死锁 1213）自动用新事务重试。
// 每次重试都是全新事务，fn 内必须重新读取数据（不要依赖上一轮的内存状态）。
func (m *Manager) TxRetry(ctx context.Context, maxAttempts int, fn func(tx *gorm.DB) error) error {
	if maxAttempts < 1 {
		return errors.New("transaction attempts must be positive")
	}
	var err error
	for i := 0; i < maxAttempts; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err = m.Tx(ctx, fn)
		if err == nil {
			return nil
		}
		if !IsRetryable(err) {
			return err
		}
		if i == maxAttempts-1 {
			break
		}
		log.WithContext(ctx).Warn("tx conflict, retrying", "attempt", i+1, "max", maxAttempts, "err", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(i+1) * retryBackoff):
		}
	}
	return err
}

// IsRetryable 判断事务失败后是否可以安全重试：
//   - errcode.IsConflict：乐观锁版本冲突 / 行竞争（事务已回滚，无副作用）
//   - MySQL 死锁错误 1213：InnoDB 自动回滚整个事务，重试安全
//     锁等待超时 1205 不重试（可能是长事务持锁，重试只会加剧排队）。
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errcode.IsConflict(err) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1213
}

// IsDuplicateErr 判断是否为唯一索引冲突，兼容 GORM 的错误翻译与包装错误。
// 用于单号/业务单号唯一索引兜底重试。
func IsDuplicateErr(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}
