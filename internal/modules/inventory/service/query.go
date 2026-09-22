package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
)

// 库存查询和跨模块库存存在性检查。

func (s *Service) List(ctx context.Context, q *dto.InventoryQuery) ([]*dto.InventoryResp, int64, error) {
	items, total, err := s.repo.List(ctx, s.tm.DB(), &repository.QueryFilter{
		WarehouseID: q.WarehouseID, LocationID: q.LocationID, SKUID: q.SKUID,
		SKUKeyword: q.SKUKeyword, InStockOnly: q.InStockOnly, Page: q.Page, Size: q.PageSize,
	})
	if err != nil {
		return nil, 0, err
	}
	return inventoryResponses(items), total, nil
}

func (s *Service) SummaryBySKU(ctx context.Context, q *dto.SummaryQuery) ([]map[string]any, int64, error) {
	return s.repo.SummaryBySKU(ctx, s.tm.DB(), q.WarehouseID, q.Page, q.PageSize)
}

func (s *Service) ListTrans(ctx context.Context, q *dto.TransQuery) ([]*dto.InventoryTransResp, int64, error) {
	items, total, err := s.repo.ListTrans(ctx, s.tm.DB(), q.InventoryID, q.OrderNo, q.TransType, q.Page, q.PageSize)
	if err != nil {
		return nil, 0, err
	}
	return inventoryTransResponses(items), total, nil
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

func inventoryResponses(items []*model.Inventory) []*dto.InventoryResp {
	resp := make([]*dto.InventoryResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &dto.InventoryResp{
			ID: item.ID, TenantID: item.TenantID, WarehouseID: item.WarehouseID,
			LocationID: item.LocationID, SKUID: item.SKUID, BatchNo: item.BatchNo,
			StockQuantity: item.StockQuantity, AvailableQty: item.AvailableQty,
			AllocatedQty: item.AllocatedQty, StockInTime: item.StockInTime,
			LocationCode: item.LocationCode, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
		})
	}
	return resp
}

func inventoryTransResponses(items []*model.InventoryTrans) []*dto.InventoryTransResp {
	resp := make([]*dto.InventoryTransResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &dto.InventoryTransResp{
			ID: item.ID, TenantID: item.TenantID, InventoryID: item.InventoryID,
			TransType: string(item.TransType), QuantityChange: item.QuantityChange,
			BeforeQuantity: item.BeforeQuantity, AfterQuantity: item.AfterQuantity,
			AvailableBefore: item.AvailableBefore, AvailableAfter: item.AvailableAfter,
			OrderNo: item.OrderNo, TaskNo: item.TaskNo, Operator: item.Operator, CreatedAt: item.CreatedAt,
		})
	}
	return resp
}
