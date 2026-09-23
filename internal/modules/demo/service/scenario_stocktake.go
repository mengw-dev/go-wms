package service

import (
	"context"
	"fmt"

	stocktakedto "gowms/internal/modules/stocktake/dto"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runStocktakeDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	operator := s.Username(ctx)
	run := newScenarioRun(
		ScenarioStocktake,
		"正在通过真实盘点 Service 执行库存快照、实盘和审核",
		ScenarioStep{Title: "生成盘点快照", Detail: "按当前库存创建盘点明细"},
		ScenarioStep{Title: "录入实盘数量", Detail: "逐条记录真实实盘数量并生成差异"},
		ScenarioStep{Title: "审核并调整", Detail: "审核差异并写入库存流水"},
	)

	var orderID int64
	var orderNo string
	if err := run.execute(0, "CREATED", "stocktake.Service.Create", func() (string, error) {
		order, err := s.stocktake.Create(ctx, &stocktakedto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			Remark:      "一键演示：库存快照、录入实盘、审核调整",
		}, operator)
		if err != nil {
			return "", err
		}
		orderID = order.ID
		orderNo = order.OrderNo
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(1, "DRAFT / 实盘已录入", "stocktake.Service.RecordActual", func() (string, error) {
		detail, err := s.stocktake.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		if len(detail.Details) == 0 {
			return "", errcode.DemoDataMissing
		}
		adjusted := false
		for i, item := range detail.Details {
			actual := item.BookQty
			if i == 0 && actual > 0 {
				actual--
				adjusted = true
			}
			if err := s.stocktake.RecordActual(ctx, orderID, item.ID, actual); err != nil {
				return "", err
			}
		}
		if !adjusted {
			return "", errcode.DemoDataMissing
		}
		return fmt.Sprintf("%s / %d 条实盘明细", orderNo, len(detail.Details)), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(2, "DRAFT → COMPLETED", "stocktake.Service.Approve", func() (string, error) {
		if err := s.stocktake.Approve(ctx, orderID, operator); err != nil {
			return "", err
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	return run.finish(fmt.Sprintf("盘点单 %s 已完成实盘和库存调整", orderNo)), nil
}
