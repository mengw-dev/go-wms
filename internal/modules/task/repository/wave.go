package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/task/model"
)

func (r *Repository) ListForWave(db *gorm.DB, orderIDs []int64, limit int, lock bool) ([]*model.Task, error) {
	list := make([]*model.Task, 0)
	if len(orderIDs) == 0 {
		return list, nil
	}
	// Cancelled attempts remain audit history; a replacement task represents the current work.
	q := db.Where("order_id IN ? AND task_type = ? AND status <> ?", orderIDs, model.TaskPick, model.TaskCancelled).Order("id").Limit(limit)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Find(&list).Error
	return list, err
}
