// Package service defines dashboard scopes and business time boundaries.
package service

import (
	"context"
	"time"

	"gowms/internal/modules/dashboard/dto"
	"gowms/internal/modules/dashboard/repository"
)

type Service struct{ repo *repository.Repository }

func New(repo *repository.Repository) *Service { return &Service{repo: repo} }

// Overview uses the project's China business date. Production stores DATETIME
// values with TZ=Asia/Shanghai and the MySQL driver's loc=Local convention.
// Counts describe current statuses; trends count order creation, not completed movements.
func (s *Service) Overview(ctx context.Context, warehouseID int64, access dto.Access, now time.Time) (*dto.Overview, error) {
	now = now.In(time.FixedZone("Asia/Shanghai", 8*60*60)).Truncate(time.Second)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := &dto.Overview{GeneratedAt: now, TodayFrom: day.Format(time.DateTime), TodayTo: now.Format(time.DateTime),
		TrendFrom: day.AddDate(0, 0, -6).Format(time.DateTime), WarehouseID: warehouseID, Access: access, Warehouses: []dto.Warehouse{}}
	if err := s.repo.Overview(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}
