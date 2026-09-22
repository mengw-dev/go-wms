package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/inbound/model"
)

func (r *Repository) CreateImportTask(ctx context.Context, db *gorm.DB, task *model.ImportTask) error {
	return db.WithContext(ctx).Create(task).Error
}

func (r *Repository) GetImportTask(ctx context.Context, db *gorm.DB, taskID string) (*model.ImportTask, error) {
	var task model.ImportTask
	if err := db.WithContext(ctx).Where("task_id = ?", taskID).First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// NextPendingImport 由后台 worker 跨租户领取待处理任务，按创建顺序扫描。
func (r *Repository) NextPendingImport(ctx context.Context, db *gorm.DB) (*model.ImportTask, error) {
	var task model.ImportTask
	res := db.WithContext(ctx).Where("status = ?", model.ImportPending).Order("created_at, id").Limit(1).Find(&task)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &task, nil
}

// ClaimImport 每次执行生成新 token；只有 PENDING 任务可以被领取。
func (r *Repository) ClaimImport(db *gorm.DB, task *model.ImportTask, token string) (bool, error) {
	res := db.Model(&model.ImportTask{}).Where("id = ? AND tenant_id = ? AND status = ?", task.ID, task.TenantID, model.ImportPending).
		Updates(map[string]any{"status": model.ImportProcessing, "run_token": token})
	return res.RowsAffected == 1, res.Error
}

func ownedImport(db *gorm.DB, task *model.ImportTask) *gorm.DB {
	return db.Model(&model.ImportTask{}).Where("id = ? AND tenant_id = ? AND status = ? AND run_token = ?", task.ID, task.TenantID, model.ImportProcessing, task.RunToken)
}

// LockImportExecution 与本行建单共用事务，防止已被收回执行权的 worker 继续写业务数据。
func (r *Repository) LockImportExecution(db *gorm.DB, task *model.ImportTask) error {
	var current model.ImportTask
	return ownedImport(db, task).Clauses(clause.Locking{Strength: "UPDATE"}).First(&current).Error
}

func (r *Repository) FinishImport(db *gorm.DB, task *model.ImportTask, status model.ImportTaskStatus, total, success, fail int, message string) (bool, error) {
	res := ownedImport(db, task).Updates(map[string]any{
		"status": status, "run_token": "", "total_rows": total, "success_rows": success, "fail_rows": fail, "error_msg": message,
	})
	return res.RowsAffected == 1, res.Error
}

func (r *Repository) TouchImport(db *gorm.DB, task *model.ImportTask) (bool, error) {
	// 同一毫秒内心跳也要产生实际更新，不能把 MySQL 的 changed rows=0 误判为失去执行权。
	res := ownedImport(db, task).Update("updated_at", gorm.Expr("GREATEST(CURRENT_TIMESTAMP(3), updated_at + INTERVAL 1000 MICROSECOND)"))
	return res.RowsAffected == 1, res.Error
}

// ReleaseImport 仅归还自己的执行权，供取消或临时故障恢复。
func (r *Repository) ReleaseImport(db *gorm.DB, task *model.ImportTask) (bool, error) {
	res := ownedImport(db, task).Updates(map[string]any{"status": model.ImportPending, "run_token": ""})
	return res.RowsAffected == 1, res.Error
}

func (r *Repository) ListStaleImports(ctx context.Context, db *gorm.DB, before time.Time, limit int) ([]*model.ImportTask, error) {
	var tasks []*model.ImportTask
	err := db.WithContext(ctx).Where("status = ? AND updated_at < ?", model.ImportProcessing, before).
		Order("updated_at, id").Limit(limit).Find(&tasks).Error
	return tasks, err
}

// ResetStaleImport 在 UPDATE 时再次检查心跳时间；扫描结果可能已经过时。
func (r *Repository) ResetStaleImport(db *gorm.DB, task *model.ImportTask, before time.Time) (bool, error) {
	res := ownedImport(db, task).Where("updated_at < ?", before).
		Updates(map[string]any{"status": model.ImportPending, "run_token": ""})
	return res.RowsAffected == 1, res.Error
}

// ListImportTasks 返回"仍有关联入库单"的历史导入任务（下拉筛选器专用，按创建时间倒序）。
// 已被删除（全删、作废）的批次会被自动过滤，避免下拉出现空批次。
func (r *Repository) ListImportTasks(ctx context.Context, db *gorm.DB, returnLimit int) ([]*model.ImportTask, error) {
	var list []*model.ImportTask
	q := db.WithContext(ctx).Model(&model.ImportTask{}).
		Where("EXISTS (SELECT 1 FROM wms_receipt_order o WHERE o.import_task_id = wms_import_task.task_id AND o.tenant_id = wms_import_task.tenant_id AND o.deleted_at IS NULL)").
		Order("created_at DESC")
	if returnLimit > 0 {
		q = q.Limit(returnLimit)
	}
	err := q.Find(&list).Error
	return list, err
}
