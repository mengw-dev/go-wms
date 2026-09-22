package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
)

// OrderDetail 盘点单详情聚合。
type OrderDetail struct {
	Order   *model.StocktakeOrder    `json:"order"`
	Details []*model.StocktakeDetail `json:"details"`
}

func (s *Service) Get(ctx context.Context, id int64) (*OrderDetail, error) {
	o, err := s.repo.GetOrder(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.StocktakeNotFound
		}
		return nil, err
	}
	details, err := s.repo.ListDetails(s.tm.DB().WithContext(ctx), id)
	if err != nil {
		return nil, err
	}
	return &OrderDetail{Order: o, Details: details}, nil
}

func (s *Service) List(ctx context.Context, q *dto.OrderQuery) ([]*model.StocktakeOrder, int64, error) {
	return s.repo.ListOrders(ctx, s.tm.DB(), q.WarehouseID, q.Status, q.Page, q.PageSize)
}
