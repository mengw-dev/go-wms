package service

import (
	"context"
	"errors"
	"fmt"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"

	"gorm.io/gorm"
)

// 库位业务。

func (s *Service) BatchCreateLocations(ctx context.Context, req *dto.LocationBatchReq) (created int, err error) {
	if err := s.ValidateWarehouse(ctx, req.WarehouseID); err != nil {
		return 0, err
	}
	if req.RowTo < req.RowFrom || req.ColTo < req.ColFrom {
		return 0, errcode.ParamError
	}
	if (req.RowTo-req.RowFrom+1)*(req.ColTo-req.ColFrom+1) > 1000 {
		return 0, errcode.LocationBatchLimit
	}
	exists, err := s.repo.ListLocationCodes(ctx, s.tm.DB(), req.WarehouseID)
	if err != nil {
		return 0, err
	}
	list := make([]*model.Location, 0, 64)
	for row := req.RowFrom; row <= req.RowTo; row++ {
		for col := req.ColFrom; col <= req.ColTo; col++ {
			code := fmt.Sprintf("%s-%02d-%02d", req.Zone, row, col)
			if _, dup := exists[code]; dup {
				continue
			}
			list = append(list, &model.Location{
				WarehouseID: req.WarehouseID, Code: code, Zone: req.Zone,
				Status: model.LocationStatusIdle,
			})
		}
	}
	if len(list) == 0 {
		return 0, nil
	}
	if err := s.repo.CreateLocationBatch(ctx, s.tm.DB(), list); err != nil {
		return 0, err
	}
	return len(list), nil
}

func (s *Service) DeleteLocation(ctx context.Context, id int64) error {
	has, err := s.stock.HasStockByLocation(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return errcode.LocationHasStock
	}
	return s.repo.DeleteLocation(ctx, s.tm.DB(), id)
}

func (s *Service) UpdateLocationStatus(ctx context.Context, id int64, status int) error {
	if status != model.LocationStatusIdle && status != model.LocationStatusDisabled && status != model.LocationStatusOccupied {
		return errcode.ParamError
	}
	return s.repo.UpdateLocation(ctx, s.tm.DB(), id, status)
}

func (s *Service) ListLocations(ctx context.Context, q *dto.LocationQuery) ([]*model.Location, int64, error) {
	return s.repo.ListLocations(ctx, s.tm.DB(), q.WarehouseID, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ValidateLocation(ctx context.Context, id int64) error {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.LocationNotFound
	}
	if l.Status == model.LocationStatusDisabled {
		return errcode.LocationDisabled
	}
	return nil
}

func (s *Service) ValidateLocationInWarehouse(ctx context.Context, warehouseID, id int64) error {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.LocationNotFound
	}
	if l.Status == model.LocationStatusDisabled {
		return errcode.LocationDisabled
	}
	if l.WarehouseID != warehouseID {
		return errcode.LocationWarehouseMismatch
	}
	return nil
}

func (s *Service) GetLocation(ctx context.Context, id int64) (*model.Location, error) {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.LocationNotFound
		}
		return nil, err
	}
	return l, nil
}

func (s *Service) UpdateLocationStatusInTx(ctx context.Context, tx *gorm.DB, id int64, status int) error {
	if err := s.repo.UpdateLocationStatusInTx(tx, id, status); err != nil {
		return err
	}
	log.WithContext(ctx).Debug("location status updated", "location_id", id, "status", status)
	return nil
}
