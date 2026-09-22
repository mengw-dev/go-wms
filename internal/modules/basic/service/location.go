package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/quota"
	pkgtx "gowms/internal/pkg/tx"
)

// 库位业务。

func (s *Service) BatchCreateLocations(ctx context.Context, req *dto.LocationBatchReq) (created int, err error) {
	if req.RowTo < req.RowFrom || req.ColTo < req.ColFrom {
		return 0, errcode.ParamError
	}
	if (req.RowTo-req.RowFrom+1)*(req.ColTo-req.ColFrom+1) > 1000 {
		return 0, errcode.LocationBatchLimit
	}
	err = s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(txDB *gorm.DB) error {
		warehouse, err := s.repo.GetWarehouseForUpdate(ctx, txDB, req.WarehouseID)
		if err != nil {
			return err
		}
		if warehouse.Status != 1 {
			return errcode.WarehouseDisabled
		}
		exists, err := s.repo.ListLocationCodes(ctx, txDB, req.WarehouseID)
		if err != nil {
			return err
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
			created = 0
			return nil
		}
		// 公开租户配额：批量建库位是本模块一次能写入最多数据的入口，按「存量 + 本批」校验。
		if err := quota.Guard(ctx, txDB, &model.Location{}, s.limits.MaxLocations, len(list), "库位"); err != nil {
			return err
		}
		if err := s.repo.CreateLocationBatch(ctx, txDB, list); err != nil {
			return err
		}
		created = len(list)
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, errcode.WarehouseNotFound
	}
	if err != nil {
		return 0, err
	}
	return created, nil
}

func (s *Service) DeleteLocation(ctx context.Context, id int64) error {
	err := s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(txDB *gorm.DB) error {
		if _, err := s.repo.GetLocationForUpdate(ctx, txDB, id); err != nil {
			return err
		}
		has, err := s.stock.HasStockByLocation(ctx, txDB, id)
		if err != nil {
			return err
		}
		if has {
			return errcode.LocationHasStock
		}
		references, err := s.repo.CountLocationReferences(ctx, txDB, id)
		if err != nil {
			return err
		}
		if references > 0 {
			return errcode.LocationHasReferences
		}
		return s.repo.DeleteLocation(ctx, txDB, id)
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errcode.LocationNotFound
	}
	return err
}

func (s *Service) UpdateLocationStatus(ctx context.Context, id int64, status int) error {
	if status != model.LocationStatusIdle && status != model.LocationStatusDisabled && status != model.LocationStatusOccupied {
		return errcode.ParamError
	}
	return s.repo.UpdateLocation(ctx, s.tm.DB(), id, status)
}

func (s *Service) ListLocations(ctx context.Context, q *dto.LocationQuery) ([]*model.Location, int64, error) {
	return s.repo.ListLocations(ctx, s.tm.DB(), q.WarehouseID, q.Zone, q.Status, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) ValidateLocation(ctx context.Context, id int64) error {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.LocationNotFound
		}
		return err
	}
	if l.Status == model.LocationStatusDisabled {
		return errcode.LocationDisabled
	}
	return nil
}

func (s *Service) ValidateLocationInWarehouse(ctx context.Context, warehouseID, id int64) error {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.LocationNotFound
		}
		return err
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
