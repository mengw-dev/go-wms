// Package idempotency 提供有副作用命令的请求级幂等：
// 以 (tenant_id, scope, idempotency_key) 为唯一键记录首次成功的执行结果，
// 相同 key + 相同指纹视为重复成功，相同 key + 不同指纹视为误用拒绝。
//
// 使用约定：
//   - Find 是成功记录的只读快路径；命中后必须比较指纹、校验响应快照再回放；
//   - 收货/上架/盘点审核在事务外预查询，避免提前固定 REPEATABLE READ 快照；
//     拣货/领取保留各自事务内的快路径及锁协议，不要求所有命令统一查询位置；
//   - 未命中才尝试执行业务并 Insert；Find 本身不锁定 key，也不授予执行权；
//   - Insert 必须与业务写入同一事务提交，业务失败时记录随事务回滚，key 可复用；
//   - 并发同 key 由唯一索引兜底；事务重试或业务拒绝后的核对遵循命令自身实现，
//     必要时先回滚再通过事务外新读核对，不能依赖同一旧快照看到并发提交；
//   - 核对查询失败应保留“结果未知”，不能伪装成确定的业务拒绝或成功；
//   - 持有事务连接时，不得为预查询或核对再从同一连接池借第二条连接。
//
// 注意：Find 的租户条件显式传入，平台 tenant_id=0 也精确匹配，
// 不依赖查询回调的旁路语义，避免跨租户 key 串用。
package idempotency

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
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

// Find 普通读取 (tenantID, scope, key) 幂等记录；未命中返回 (nil, nil)。
// 传入事务时遵循该事务的读视图，不是锁定读或强制最新读。
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

// RequestFingerprint 同时持有新写入值与安全的旧格式候选，供快路径和回滚后核对一致使用。
// 字段不导出，避免把旧格式候选写入数据库。
type RequestFingerprint struct {
	value  string
	legacy string
}

// String 返回供 Record.RequestHash 保存的 v2 指纹（46 字节，兼容现有 VARCHAR(64)）。
func (f RequestFingerprint) String() string { return f.value }

// Matches 比较已保存的指纹。旧格式仅在固定字段数且所有字段不含分隔符时兼容；
// 含分隔符的旧记录无法通用地证明字段边界，拒绝回放而不是冒险认作成功。
// 调用方必须保持各 scope 的字段数、顺序与既有业务规范化规则不变。
func (f RequestFingerprint) Matches(stored string) bool {
	return f.value != "" && (stored == f.value || (f.legacy != "" && stored == f.legacy))
}

// Fingerprint 对字段数、各字段字节长度与原始字节进行 SHA-256，消除分隔符、空字段
// 及任意二进制/UTF-8 内容带来的编码歧义；v2 前缀与旧 64 位十六进制格式明确区分。
// 不要包含时间戳、随机数等每次请求都会变化的字段。
func Fingerprint(parts ...string) RequestFingerprint {
	payload := binary.BigEndian.AppendUint64([]byte("gowms:idempotency:v2\x00"), uint64(len(parts)))
	legacySafe := len(parts) > 0
	for _, part := range parts {
		payload = binary.BigEndian.AppendUint64(payload, uint64(len(part)))
		payload = append(payload, part...)
		legacySafe = legacySafe && !strings.Contains(part, "\x1f")
	}
	sum := sha256.Sum256(payload)
	f := RequestFingerprint{value: "v2:" + base64.RawURLEncoding.EncodeToString(sum[:])}
	if legacySafe {
		legacySum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
		f.legacy = hex.EncodeToString(legacySum[:])
	}
	return f
}

// EmptySuccessJSON 无返回值命令（HTTP 层 data:null）的成功回放固定表示：
// 写入固定的 "{}" 空对象，回放时由 ValidateEmptySuccess 校验为空对象。
// 用空对象区分「合法空成功」与「损坏记录」，
// 不通过 ResultJSON 为空串或 "null" 表达成功。
const EmptySuccessJSON = "{}"

// ValidateEmptySuccess 校验空成功回放标记：
// 仅接受无字段的 JSON 对象（允许空白）；空串、null、含字段或非法 JSON 均视为损坏。
func ValidateEmptySuccess(raw string) error {
	fields, err := decodeResultFields(raw)
	if err != nil {
		return err
	}
	if len(fields) != 0 {
		return errcode.Internal
	}
	return nil
}

// decodeResultFields 解析回放 JSON 的顶层字段对象；空串/null/非法 JSON/非对象均视为损坏。
// 供各命令共享「能解析不等于有效」的判定口径。
func decodeResultFields(raw string) (map[string]json.RawMessage, error) {
	if raw == "" {
		return nil, errcode.Internal
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return nil, errcode.Wrap(err, errcode.Internal)
	}
	if fields == nil {
		return nil, errcode.Internal
	}
	return fields, nil
}
