package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
)

// 库存查询和跨模块库存存在性检查。

func (s *Service) List(ctx context.Context, q *dto.InventoryQuery) ([]*model.Inventory, int64, error) {
	return s.repo.List(ctx, s.tm.DB(), &repository.QueryFilter{
		WarehouseID: q.WarehouseID, LocationID: q.LocationID, SKUID: q.SKUID,
		SKUKeyword: q.SKUKeyword, InStockOnly: q.InStockOnly, Page: q.Page, Size: q.PageSize,
	})
}

func (s *Service) SummaryBySKU(ctx context.Context, q *dto.SummaryQuery) ([]map[string]any, int64, error) {
	return s.repo.SummaryBySKU(ctx, s.tm.DB(), q.WarehouseID, q.Page, q.PageSize)
}

func (s *Service) ListTrans(ctx context.Context, q *dto.TransQuery) ([]*model.InventoryTrans, int64, error) {
	return s.repo.ListTrans(ctx, s.tm.DB(), q.InventoryID, q.OrderNo, q.TransType, q.Page, q.PageSize)
}

func (s *Service) HasStockByWarehouse(ctx context.Context, db *gorm.DB, warehouseID int64) (bool, error) {
	return s.repo.HasStockByWarehouse(ctx, db, warehouseID)
}

func (s *Service) HasStockByLocation(ctx context.Context, db *gorm.DB, locationID int64) (bool, error) {
	return s.repo.HasStockByLocation(ctx, db, locationID)
}

func (s *Service) HasStockBySKU(ctx context.Context, db *gorm.DB, skuID int64) (bool, error) {
	return s.repo.HasStockBySKU(ctx, db, skuID)
}
