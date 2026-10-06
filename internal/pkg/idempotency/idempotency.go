// Package idempotency 提供有副作用命令的请求级幂等：
// 以 (tenant_id, scope, idempotency_key) 为唯一键记录首次成功的执行结果，
// 相同 key + 相同指纹视为重复成功，相同 key + 不同指纹视为误用拒绝。
//
// 使用约定（收货/上架/拣货/盘点审核共用同一套）：
//   - 调用方在业务事务内先 Find；未命中则执行业务，最后 Insert；
//   - Insert 必须与业务写入同一事务提交，业务失败时记录随事务回滚，key 可复用；
//   - 并发同 key 由唯一索引兜底：后到者插入冲突返回可重试冲突，
//     重试后 Find 命中已有记录，回放首次结果。
//
// 注意：Find 的租户条件显式传入，平台 tenant_id=0 也精确匹配，
// 不依赖查询回调的旁路语义，避免跨租户 key 串用。
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
)

// Record 幂等记录：保存一次成功执行的响应快照，供重试回放。
type Record struct {
	ID             int64  `gorm:"primaryKey"`
	TenantID       int64  `gorm:"not null;default:0;uniqueIndex:uk_idem_tenant_scope_key,priority:1"`
	Scope          string `gorm:"size:64;not null;uniqueIndex:uk_idem_tenant_scope_key,priority:2"`
	IdempotencyKey string `gorm:"size:64;not null;uniqueIndex:uk_idem_tenant_scope_key,priority:3"`
	RequestHash    string `gorm:"size:64;not null"`
	// ObjectID 关联的业务对象（如拣货任务 id），仅用于排查。
	ObjectID int64 `gorm:"not null;default:0"`
	// ResultJSON 首次成功时的响应快照，非空；重试命中时原样回放（不重新读库），
	// 缺失或损坏按内部错误处理，不用当前进度兜底。
	ResultJSON string `gorm:"type:text;not null"`
	// CreatedAt 建索引：清理 Worker 按 created_at 删除，避免全表扫描。
	// AutoMigrate 与迁移 000009 保持一致（idx_idem_created_at）。
	CreatedAt time.Time `gorm:"index:idx_idem_created_at"`
}

func (Record) TableName() string { return "wms_idempotency" }

// Find 查询 (tenantID, scope, key) 幂等记录；未命中返回 (nil, nil)。
func Find(db *gorm.DB, tenantID int64, scope, key string) (*Record, error) {
	var record Record
	err := db.Where("tenant_id = ? AND scope = ? AND idempotency_key = ?", tenantID, scope, key).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// Insert 写入幂等记录；唯一键冲突（并发同 key）映射为可重试的 errcode.Conflict。
func Insert(db *gorm.DB, record *Record) error {
	if err := db.Create(record).Error; err != nil {
		if isDuplicateKey(err) {
			return errcode.Conflict
		}
		return err
	}
	return nil
}

// isDuplicateKey 判断唯一索引冲突：兼容 GORM 翻译错误与 MySQL 1062。
func isDuplicateKey(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

// Fingerprint 计算请求指纹：对业务语义字段（如 task_id + qty）做 SHA-256。
// 不要包含时间戳、随机数等每次请求都会变化的字段。
func Fingerprint(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}
