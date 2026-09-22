package service

import (
	"context"
	"fmt"

	outbounddto "gowms/internal/modules/outbound/dto"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runOutboundDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const qty = 20
	operator := s.Username(ctx)
	order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		BizOrderNo:  demoBizOrderNo(1),
		Remark:      "一键演示：出库审核、FIFO 分配、拣货出库",
		Details: []outbounddto.OrderDetailItem{{
			SKUID: refs.SKU.ID, ExpectedQty: qty,
		}},
	}, operator)
	if err != nil {
		return nil, err
	}
	if err := s.outbound.Submit(ctx, order.ID); err != nil {
		return nil, err
	}
	if err := s.outbound.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	detail, err := s.outbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	var picked int
	for _, t := range detail.Tasks {
		if t.TaskType != taskmodel.TaskPick || t.Status == taskmodel.TaskCompleted {
			continue
		}
		pickQty := t.TargetQty - t.DoneQty
		if pickQty <= 0 {
			continue
		}
		if err := s.outbound.Pick(ctx, t.ID, pickQty, operator, nil); err != nil {
			return nil, err
		}
		picked += pickQty
	}
	if picked != qty {
		return nil, errcode.DemoDataMissing
	}

	return &ScenarioResult{
		Name:    ScenarioOutbound,
		Summary: fmt.Sprintf("出库单 %s 已完成 FIFO 分配并拣货 %d 件", order.OrderNo, qty),
		Steps: []ScenarioStep{
			{Title: "创建出库单", Detail: order.OrderNo},
			{Title: "提交并审核", Detail: "按入库时间 FIFO 分配库存并生成拣货任务"},
			{Title: "完成拣货", Detail: fmt.Sprintf("货品 %s，数量 %d，库存已扣减", refs.SKU.Code, qty)},
		},
	}, nil
}
