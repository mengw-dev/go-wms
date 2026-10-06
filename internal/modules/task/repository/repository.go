package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/task/model"
	"gowms/internal/pkg/dbutil"
)

type Repository struct{}

func New() *Repository { return &Repository{} }

// CreateBatch 在事务内批量创建任务。
func (r *Repository) CreateBatch(tx *gorm.DB, tasks []*model.Task) error {
	return tx.CreateInBatches(tasks, 100).Error
}

// GetForUpdate 事务内悲观行锁锁定任务。
func (r *Repository) GetForUpdate(tx *gorm.DB, id int64) (*model.Task, error) {
	var t model.Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&t, id).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// UpdateProgress 推进任务状态与完成量（version 乐观锁防护）。
// 返回 RowsAffected：0 表示版本冲突（任务已被并发事务推进），调用方应返回冲突错误。
func (r *Repository) UpdateProgress(tx *gorm.DB, t *model.Task) (int64, error) {
	res := tx.Model(&model.Task{}).Where("id = ? AND version = ?", t.ID, t.Version).Updates(map[string]any{
		"done_qty": t.DoneQty, "status": t.Status, "operator": t.Operator, "version": t.Version + 1,
	})
	return res.RowsAffected, res.Error
}

// GetByDetailForUpdate 事务内按明细锁定任务（收货/上架按明细推进）。
func (r *Repository) GetByDetailForUpdate(tx *gorm.DB, orderID, detailID int64, taskType model.TaskType) (*model.Task, error) {
	var t model.Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ? AND detail_id = ? AND task_type = ?", orderID, detailID, taskType).
		First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// CountUnfinished 统计单据下未完成任务数。
func (r *Repository) CountUnfinished(tx *gorm.DB, orderID int64, taskType model.TaskType) (int64, error) {
	var n int64
	err := tx.Model(&model.Task{}).
		Where("order_id = ? AND task_type = ? AND status <> ?", orderID, taskType, model.TaskCompleted).
		Count(&n).Error
	return n, err
}

// ListByOrderForUpdate 按单据升序锁定全部任务行：取消前校验是否已有任务开工，
// 并与拣货在同一把行锁上互斥（拣货与取消都先锁任务行）。
func (r *Repository) ListByOrderForUpdate(tx *gorm.DB, orderID int64) ([]*model.Task, error) {
	var list []*model.Task
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("order_id = ?", orderID).Order("id").Find(&list).Error
	return list, err
}

// CancelByOrder 取消单据下所有未完成任务。
func (r *Repository) CancelByOrder(tx *gorm.DB, orderID int64) error {
	return tx.Model(&model.Task{}).
		Where("order_id = ? AND status IN ?", orderID, []model.TaskStatus{model.TaskCreated, model.TaskInProgress}).
		Updates(map[string]any{
			"status":  model.TaskCancelled,
			"version": gorm.Expr("version + 1"),
		}).Error
}

// Claim 条件领取/续领拣货任务的作业租约：任务可作业（CREATED/IN_PROGRESS），
// 且无租约、租约已过期或本人持有时更新成功。
// 返回 RowsAffected：0 表示被他人持有且未过期（或任务状态不允许）。
func (r *Repository) Claim(tx *gorm.DB, taskID int64, operator, token string, expireAt time.Time) (int64, error) {
	now := time.Now()
	res := tx.Model(&model.Task{}).
		Where("id = ? AND task_type = ?", taskID, model.TaskPick).
		Where("status IN ?", []model.TaskStatus{model.TaskCreated, model.TaskInProgress}).
		Where("(lease_expire_at IS NULL OR lease_expire_at < ? OR claimed_by = ?)", now, operator).
		Updates(map[string]any{
			"claimed_by": operator, "claim_token": token, "lease_expire_at": expireAt,
		})
	return res.RowsAffected, res.Error
}

// RenewClaim 续租：凭证一致时延长租约（调用方已持有任务行锁并校验过凭证）。
// 返回 RowsAffected：0 表示新到期时间与旧值相同，属正常情况（变更未发生）。
func (r *Repository) RenewClaim(tx *gorm.DB, taskID int64, token string, expireAt time.Time) (int64, error) {
	res := tx.Model(&model.Task{}).Where("id = ? AND claim_token = ?", taskID, token).
		Update("lease_expire_at", expireAt)
	return res.RowsAffected, res.Error
}

func (r *Repository) Get(ctx context.Context, db *gorm.DB, id int64) (*model.Task, error) {
	var t model.Task
	if err := db.WithContext(ctx).First(&t, id).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) List(ctx context.Context, db *gorm.DB, orderID int64, taskType, status, keyword string, page, size int) ([]*model.Task, int64, error) {
	q := db.WithContext(ctx).Model(&model.Task{})
	if orderID > 0 {
		q = q.Where("order_id = ?", orderID)
	}
	if taskType != "" {
		q = q.Where("task_type = ?", taskType)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword != "" {
		q = q.Where("task_no LIKE ? OR order_no LIKE ?", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Task
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}
