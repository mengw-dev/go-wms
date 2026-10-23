package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
)

// ValidateWarehouseInTx 共享锁定并校验仓库，阻止并发删除/禁用，同时允许多个订单建单。
func (s *Service) ValidateWarehouseInTx(ctx context.Context, tx *gorm.DB, id int64) error {
	warehouse, err := s.repo.GetWarehouseForShare(ctx, tx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errcode.WarehouseNotFound
	}
	if err != nil {
		return err
	}
	if warehouse.Status != 1 {
		return errcode.WarehouseDisabled
	}
	return nil
}

// GetSKUsByIDsForShare 在仓库锁之后按主键顺序锁住 SKU，并返回锁内的创建快照。
// 与 DeleteSKU 共用行锁；只检查存在性，保留原 GetSKUsByIDs 的状态口径。
func (s *Service) GetSKUsByIDsForShare(ctx context.Context, tx *gorm.DB, ids []int64) (map[int64]*model.SKU, error) {
	result, err := s.repo.GetSKUsByIDsForShare(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if result[id] == nil {
			return nil, errcode.SKUNotFound
		}
	}
	return result, nil
}

// ValidateLocationInWarehouseInTx 依库存写入的仓库→库位顺序加锁，
// 对锁内当前状态校验，防止上架覆盖已经提交的禁用。锁保留到业务提交。
func (s *Service) ValidateLocationInWarehouseInTx(ctx context.Context, tx *gorm.DB, warehouseID, id int64) error {
	if _, err := s.repo.GetWarehouseForUpdate(ctx, tx, warehouseID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.WarehouseNotFound
		}
		return err
	}
	location, err := s.repo.GetLocationForUpdate(ctx, tx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errcode.LocationNotFound
	}
	if err != nil {
		return err
	}
	if location.Status == model.LocationStatusDisabled {
		return errcode.LocationDisabled
	}
	if location.WarehouseID != warehouseID {
		return errcode.LocationWarehouseMismatch
	}
	return nil
}

// GetLocationInTx returns the locked catalog snapshot for a business receipt.
func (s *Service) GetLocationInTx(ctx context.Context, tx *gorm.DB, id int64) (*model.Location, error) {
	row, e := s.repo.GetLocationForUpdate(ctx, tx, id)
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, errcode.LocationNotFound
	}
	return row, e
}
