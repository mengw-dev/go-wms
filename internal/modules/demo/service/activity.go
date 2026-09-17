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

// ActivitySnapshot 演示账号可见的业务与操作记录。
type ActivitySnapshot struct {
	Operations      []*sysmodel.SysOperLog           `json:"operations"`
	InboundOrders   []*inboundmodel.ReceiptOrder     `json:"inbound_orders"`
	OutboundOrders  []*outboundmodel.ShipmentOrder   `json:"outbound_orders"`
	StocktakeOrders []*stocktakemodel.StocktakeOrder `json:"stocktake_orders"`
	Tasks           []*taskmodel.Task                `json:"tasks"`
	InventoryTrans  []*invmodel.InventoryTrans       `json:"inventory_trans"`
}

// Activity 查询当前演示账号最近发生的操作和业务记录。
func (s *Service) Activity(ctx context.Context, limit int) (*ActivitySnapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	username := s.Username()
	result := &ActivitySnapshot{}

	if err := s.db.WithContext(ctx).
		Where("username = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&result.Operations).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("created_by = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&result.InboundOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("created_by = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&result.OutboundOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("created_by = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&result.StocktakeOrders).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("operator = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&result.Tasks).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("operator = ?", username).
		Order("created_at DESC").
		Limit(limit).
		Find(&result.InventoryTrans).Error; err != nil {
		return nil, err
	}
	return result, nil
}
