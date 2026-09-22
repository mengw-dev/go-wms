package repository

import (
	"context"

	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/dbutil"
)

func (r *Repository) InsertOperLogs(ctx context.Context, logs []*model.SysOperLog) error {
	if len(logs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).CreateInBatches(logs, 100).Error
}

func (r *Repository) ListOperLogs(ctx context.Context, username, path string, page, size int) ([]*model.SysOperLog, int64, error) {
	q := r.db.WithContext(ctx).Model(&model.SysOperLog{})
	if username != "" {
		q = q.Where("username = ?", username)
	}
	if path != "" {
		q = q.Where("path LIKE ?", "%"+dbutil.LikePattern(path)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SysOperLog
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}
