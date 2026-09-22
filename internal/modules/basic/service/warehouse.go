package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/quota"
	pkgtx "gowms/internal/pkg/tx"
)

// 仓库业务。

func (s *Service) CreateWarehouse(ctx context.Context, req *dto.WarehouseReq) error {
	// 公开租户配额：防止访客无限建仓库。
	if err := quota.Guard(ctx, s.tm.DB(), &model.Warehouse{}, s.limits.MaxWarehouses, 1, "仓库"); err != nil {
		return err
	}
	if _, err := s.repo.GetWarehouseByCode(ctx, s.tm.DB(), req.Code); err == nil {
		return errcode.WarehouseExist
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.repo.CreateWarehouse(ctx, s.tm.DB(), &model.Warehouse{
		Code: req.Code, Name: req.Name, Remark: req.Remark, Status: 1,
	})
}

func (s *Service) UpdateWarehouse(ctx context.Context, id int64, req *dto.WarehouseReq) error {
	return s.repo.UpdateWarehouse(ctx, s.tm.DB(), id, req.Name, req.Remark)
}

func (s *Service) UpdateWarehouseStatus(ctx context.Context, id int64, status int) error {
	if status != 0 && status != 1 {
		return errcode.ParamError
	}
	return s.repo.UpdateWarehouseStatus(ctx, s.tm.DB(), id, status)
}

func (s *Service) DeleteWarehouse(ctx context.Context, id int64) error {
	err := s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(txDB *gorm.DB) error {
		if _, err := s.repo.GetWarehouseForUpdate(ctx, txDB, id); err != nil {
			return err
		}
		has, err := s.stock.HasStockByWarehouse(ctx, txDB, id)
		if err != nil {
			return err
		}
		if has {
			return errcode.WarehouseHasStock
		}
		references, err := s.repo.CountWarehouseReferences(ctx, txDB, id)
		if err != nil {
			return err
		}
		if references > 0 {
			return errcode.WarehouseHasReferences
		}
		return s.repo.DeleteWarehouse(ctx, txDB, id)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errcode.WarehouseNotFound
	}
	return err
}

func (s *Service) ListWarehouses(ctx context.Context, q *dto.WarehouseQuery) ([]*model.Warehouse, int64, error) {
	return s.repo.ListWarehouses(ctx, s.tm.DB(), q.Keyword, q.Status, q.Page, q.PageSize)
}

func (s *Service) ValidateWarehouse(ctx context.Context, id int64) error {
	w, err := s.repo.GetWarehouse(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.WarehouseNotFound
		}
		return err
	}
	if w.Status != 1 {
		return errcode.WarehouseDisabled
	}
	return nil
}

func (s *Service) GetWarehouseByCode(ctx context.Context, code string) (*model.Warehouse, error) {
	w, err := s.repo.GetWarehouseByCode(ctx, s.tm.DB(), code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.WarehouseNotFound
		}
		return nil, err
	}
	return w, nil
}
