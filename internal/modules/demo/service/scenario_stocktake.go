package service

import (
	"context"
	"fmt"

	stocktakedto "gowms/internal/modules/stocktake/dto"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runStocktakeDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	operator := s.Username(ctx)
	order, err := s.stocktake.Create(ctx, &stocktakedto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		Remark:      "一键演示：库存快照、录入实盘、审核调整",
	}, operator)
	if err != nil {
		return nil, err
	}
	detail, err := s.stocktake.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	if len(detail.Details) == 0 {
		return nil, errcode.DemoDataMissing
	}

	adjusted := false
	for i, item := range detail.Details {
		actual := item.BookQty
		if i == 0 && actual > 0 {
			actual--
			adjusted = true
		}
		if err := s.stocktake.RecordActual(ctx, order.ID, item.ID, actual); err != nil {
			return nil, err
		}
	}
	if !adjusted {
		return nil, errcode.DemoDataMissing
	}
	if err := s.stocktake.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	return &ScenarioResult{
		Name:    ScenarioStocktake,
		Summary: fmt.Sprintf("盘点单 %s 已完成实盘和库存调整", order.OrderNo),
		Steps: []ScenarioStep{
			{Title: "生成盘点快照", Detail: order.OrderNo},
			{Title: "录入实盘数量", Detail: fmt.Sprintf("共 %d 条库存明细", len(detail.Details))},
			{Title: "审核并调整", Detail: "差异数量已写入库存流水"},
		},
	}, nil
}
