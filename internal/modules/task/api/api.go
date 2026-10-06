// Package api 定义统一任务模块提供给入库和出库流程的操作接口。
package api

import (
	"context"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/task/model"
)

// CreateTask 创建任务入参。
type CreateTask struct {
	TaskType     model.TaskType
	OrderID      int64
	OrderNo      string
	DetailID     int64 // 单据明细 id（收货/上架/拣货任务均写入）
	AllocationID int64 // 拣货任务对应的分配行
	SKUID        int64
	WarehouseID  int64
	// 拣货任务的作业位置（来自分配行），收货/上架任务留空。
	LocationID   int64
	LocationCode string
	BatchNo      string
	TargetQty    int
}

// DetailTaskPageSize 单据详情页加载该单全部任务时使用的页大小（任务数不会很大，一次取足）。
const DetailTaskPageSize = 200

// TaskAPI task 模块对外接口：inbound / outbound 通过它操作统一任务表。
type TaskAPI interface {
	// Create 在业务事务内批量创建任务。
	Create(ctx context.Context, tx *gorm.DB, tasks []*CreateTask) error
	// AddProgress 在业务事务内累加任务完成量，自动推进 CREATED → IN_PROGRESS → COMPLETED；
	// 调用方必须先在同一事务内通过 GetForUpdate 锁定该任务，本方法不再自行锁读。
	AddProgress(ctx context.Context, tx *gorm.DB, task *model.Task, qty int, operator string) error
	// AddProgressByDetail 按单据明细定位任务并累加完成量（收货/上架按明细推进）。
	AddProgressByDetail(ctx context.Context, tx *gorm.DB, orderID, detailID int64, taskType model.TaskType, qty int, operator string) error
	// CountUnfinished 统计单据下未完成任务数（判断单据是否可流转完成）。
	CountUnfinished(ctx context.Context, tx *gorm.DB, orderID int64, taskType model.TaskType) (int64, error)
	// ListByOrderForUpdate 事务内按单据升序锁定全部任务行：取消前校验是否已有任务开工，
	// 并与拣货在任务行上互斥（双方都先锁任务行）。
	ListByOrderForUpdate(ctx context.Context, tx *gorm.DB, orderID int64) ([]*model.Task, error)
	// CancelByOrder 取消单据下所有未完成任务（同事务调用）。
	CancelByOrder(ctx context.Context, tx *gorm.DB, orderID int64) error
	// GetForUpdate 事务内行锁读取任务：执行拣货/上架前在同一事务内锁定并校验任务状态，
	// 同时作为 AddProgress 的前置锁（AddProgress 复用调用方持有的任务行锁）。
	GetForUpdate(ctx context.Context, tx *gorm.DB, taskID int64) (*model.Task, error)
	// Claim 条件领取/续领任务作业租约（拣货 PDA）：仅当任务可作业且
	// 无租约 / 租约已过期 / 本人持有时更新成功，返回受影响行数。
	Claim(ctx context.Context, tx *gorm.DB, taskID int64, operator, token string, expireAt time.Time) (int64, error)
	// RenewClaim 延长本人持有的任务租约（拣货成功后调用，任务行已锁定）。
	RenewClaim(ctx context.Context, tx *gorm.DB, taskID int64, token string, expireAt time.Time) (int64, error)
	// List 查询任务（只读，使用非事务连接）。
	List(ctx context.Context, orderID int64, taskType, status, keyword string, page, size int) ([]*model.Task, int64, error)
	Get(ctx context.Context, taskID int64) (*model.Task, error)
}
