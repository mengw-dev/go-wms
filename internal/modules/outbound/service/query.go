package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

// OrderDetail 出库单详情聚合。
type OrderDetail struct {
	Order       *model.ShipmentOrder         `json:"order"`
	Details     []*model.ShipmentOrderDetail `json:"details"`
	Allocations []*model.Allocation          `json:"allocations"`
	Tasks       []*taskmodel.Task            `json:"tasks"`
}

func (s *Service) Get(ctx context.Context, id int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrder(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ShipOrderNotFound
		}
		return nil, err
	}
	details, err := s.repo.ListDetails(s.tm.DB().WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	allocations, err := s.repo.ListAllocations(s.tm.DB().WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	tasks, _, err := s.taskAPI.List(ctx, id, "", "", "", 1, taskapi.DetailTaskPageSize)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{Order: o, Details: details, Allocations: allocations, Tasks: tasks}, nil
}

func (s *Service) List(ctx context.Context, q *dto.OrderQuery) ([]*model.ShipmentOrder, int64, error) {
	return s.repo.ListOrders(ctx, s.tm.DB(), q.WarehouseID, q.Status, q.Keyword, q.CreatedAtFrom, q.CreatedAtTo, q.Page, q.PageSize)
}
