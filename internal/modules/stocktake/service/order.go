package service

import (
	"context"

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
			LocationCode: req.LocationCode, Status: model.OrderDraft,
			Remark: req.Remark, CreatedBy: operator,
		}
		return s.repo.CreateOrder(tx, order, details)
	})
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) Cancel(ctx context.Context, orderID int64) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		if _, err := s.repo.GetOrderForUpdate(tx, orderID); err != nil {
			return errcode.StocktakeNotFound
		}
		if n, err := s.repo.UpdateStatus(tx, orderID, model.OrderDraft, model.OrderCancelled); err != nil {
			return err
		} else if n == 0 {
			return errcode.StocktakeVersionBad
		}
		return nil
	})
}
