// Package service 提供盘点快照、实盘录入和审核调整业务规则。
package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/quota"
	"gowms/internal/pkg/snowflake"
)

// 盘点单创建与取消。

func (s *Service) Create(ctx context.Context, req *dto.CreateOrderReq, operator string) (*model.StocktakeOrder, error) {
	// 公开租户配额：盘点单明细数由当前库存行数决定，防访客反复建单堆积。
	if err := quota.Guard(ctx, s.tm.DB(), &model.StocktakeOrder{}, s.limits.MaxStocktakeOrders, 1, "盘点单"); err != nil {
		return nil, err
	}
	var order *model.StocktakeOrder
	err := s.tm.Tx(ctx, func(tx *gorm.DB) error {
		details, err := s.repo.SnapshotInventory(tx, req.WarehouseID, req.LocationID)
		if err != nil {
			return err
		}
		if len(details) == 0 {
			return errcode.StocktakeNoDetail
		}
		for _, d := range details {
			d.ID = snowflake.Next()
		}
		order = &model.StocktakeOrder{
			Base: sysmodel.Base{ID: snowflake.Next()}, OrderNo: s.no.Next(ctx, "PD"),
			WarehouseID: req.WarehouseID, LocationID: req.LocationID,
			Status: model.OrderDraft,
			Remark: req.Remark, CreatedBy: operator,
		}
		if req.LocationID > 0 {
			order.LocationCode = details[0].LocationCode
		}
		return s.repo.CreateOrder(tx, order, details)
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

// CreateResponse 返回创建盘点单的 HTTP 响应结构。
func (s *Service) CreateResponse(ctx context.Context, req *dto.CreateOrderReq, operator string) (*dto.OrderResp, error) {
	order, err := s.Create(ctx, req, operator)
	if err != nil {
		return nil, err
	}
	return orderResponse(order), nil
}
func (s *Service) Cancel(ctx context.Context, orderID int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.StocktakeNotFound
			}
			return err
		}
		if !model.CanTransit(o.Status, model.OrderCancelled) {
			return errcode.StocktakeStatusWrong
		}
		if n, err := s.repo.UpdateStatus(tx, orderID, o.Status, model.OrderCancelled); err != nil {
			return err
		} else if n == 0 {
			return errcode.StocktakeVersionBad
		}
		return nil
	})
}
