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

// GetResponse 返回盘点详情的 HTTP 响应结构。
func (s *Service) GetResponse(ctx context.Context, id int64) (*dto.OrderDetailResp, error) {
	detail, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return &dto.OrderDetailResp{Order: orderResponse(detail.Order), Details: detailResponses(detail.Details)}, nil
}

// ListResponses 返回盘点单列表的 HTTP 响应结构。
func (s *Service) ListResponses(ctx context.Context, q *dto.OrderQuery) ([]*dto.OrderResp, int64, error) {
	orders, total, err := s.List(ctx, q)
	if err != nil {
		return nil, 0, err
	}
	resp := make([]*dto.OrderResp, 0, len(orders))
	for _, order := range orders {
		resp = append(resp, orderResponse(order))
	}
	return resp, total, nil
}

func orderResponse(order *model.StocktakeOrder) *dto.OrderResp {
	return &dto.OrderResp{
		ID: order.ID, TenantID: order.TenantID, OrderNo: order.OrderNo,
		WarehouseID: order.WarehouseID, LocationID: order.LocationID, LocationCode: order.LocationCode,
		Status: string(order.Status), Remark: order.Remark, CreatedBy: order.CreatedBy,
		CreatedAt: order.CreatedAt, UpdatedAt: order.UpdatedAt,
	}
}

func detailResponses(details []*model.StocktakeDetail) []*dto.DetailResp {
	resp := make([]*dto.DetailResp, 0, len(details))
	for _, detail := range details {
		resp = append(resp, detailResponse(detail))
	}
	return resp
}

func detailResponse(detail *model.StocktakeDetail) *dto.DetailResp {
	return &dto.DetailResp{
		ID: detail.ID, TenantID: detail.TenantID, OrderID: detail.OrderID, InventoryID: detail.InventoryID,
		SKUID: detail.SKUID, SKUCode: detail.SKUCode, SKUName: detail.SKUName,
		LocationID: detail.LocationID, LocationCode: detail.LocationCode, BatchNo: detail.BatchNo,
		BookQty: detail.BookQty, ActualQty: detail.ActualQty, DiffQty: detail.DiffQty, Adjusted: detail.Adjusted,
		CreatedAt: detail.CreatedAt, UpdatedAt: detail.UpdatedAt,
	}
}
