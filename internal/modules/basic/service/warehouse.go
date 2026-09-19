package service

import (
	"context"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/quota"
)

// 仓库业务。

func (s *Service) CreateWarehouse(ctx context.Context, req *dto.WarehouseReq) error {
	// 公开租户配额：防止访客无限建仓库。
	if err := quota.Guard(ctx, s.tm.DB(), &model.Warehouse{}, s.limits.MaxWarehouses, 1, "仓库"); err != nil {
		return err
	}
	if _, err := s.repo.GetWarehouseByCode(ctx, s.tm.DB(), req.Code); err == nil {
		return errcode.WarehouseExist
	}
	return s.repo.CreateWarehouse(ctx, s.tm.DB(), &model.Warehouse{
		Code: req.Code, Name: req.Name, Remark: req.Remark, Status: 1,
	})
}

func (s *Service) UpdateWarehouse(ctx context.Context, id int64, req *dto.WarehouseReq, status *int) error {
	return s.repo.UpdateWarehouse(ctx, s.tm.DB(), id, req.Name, req.Remark, status)
}

func (s *Service) DeleteWarehouse(ctx context.Context, id int64) error {
	has, err := s.stock.HasStockByWarehouse(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return errcode.WarehouseHasStock
	}
	n, err := s.repo.CountLocationsByWarehouse(ctx, s.tm.DB(), id)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.WarehouseHasReferences
	}
	return s.repo.DeleteWarehouse(ctx, s.tm.DB(), id)
}

func (s *Service) ListWarehouses(ctx context.Context, q *dto.WarehouseQuery) ([]*model.Warehouse, int64, error) {
	return s.repo.ListWarehouses(ctx, s.tm.DB(), q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ValidateWarehouse(ctx context.Context, id int64) error {
	w, err := s.repo.GetWarehouse(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.WarehouseNotFound
	}
	if w.Status != 1 {
		return errcode.WarehouseDisabled
	}
	return nil
}

func (s *Service) GetWarehouseByCode(ctx context.Context, code string) (*model.Warehouse, error) {
	w, err := s.repo.GetWarehouseByCode(ctx, s.tm.DB(), code)
	if err != nil {
		return nil, errcode.WarehouseNotFound
	}
	return w, nil
}
