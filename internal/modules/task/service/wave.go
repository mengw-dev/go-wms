package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) ListForWave(ctx context.Context, db *gorm.DB, orderIDs []int64, limit int, lock bool) ([]*model.Task, error) {
	if len(orderIDs) > 50 || limit < 1 || limit > 1001 {
		return nil, errcode.ParamError
	}
	return s.repo.ListForWave(db.WithContext(ctx), orderIDs, limit, lock)
}
