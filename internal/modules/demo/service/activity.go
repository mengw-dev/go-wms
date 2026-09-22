package service

import (
	"context"

	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
)

// Activity 查询当前演示账号最近发生的操作和业务记录。
func (s *Service) Activity(ctx context.Context, limit int) (*ActivitySnapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	username := s.Username(ctx)

	var operations []*sysmodel.SysOperLog
	if err := s.db.WithContext(ctx).
		Where("username = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&operations).Error; err != nil {
		return nil, err
	}

	var inboundOrders []*inboundmodel.ReceiptOrder
	if err := s.db.WithContext(ctx).
		Where("created_by = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&inboundOrders).Error; err != nil {
		return nil, err
	}

	var outboundOrders []*outboundmodel.ShipmentOrder
	if err := s.db.WithContext(ctx).
		Where("created_by = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&outboundOrders).Error; err != nil {
		return nil, err
	}

	var stocktakeOrders []*stocktakemodel.StocktakeOrder
	if err := s.db.WithContext(ctx).
		Where("created_by = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&stocktakeOrders).Error; err != nil {
		return nil, err
	}

	var tasks []*taskmodel.Task
	if err := s.db.WithContext(ctx).
		Where("operator = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&tasks).Error; err != nil {
		return nil, err
	}

	var inventoryTrans []*invmodel.InventoryTrans
	if err := s.db.WithContext(ctx).
		Where("operator = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&inventoryTrans).Error; err != nil {
		return nil, err
	}

	return activitySnapshot(operations, inboundOrders, outboundOrders, stocktakeOrders, tasks, inventoryTrans), nil
}
