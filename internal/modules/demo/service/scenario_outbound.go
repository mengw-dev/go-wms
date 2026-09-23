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
	run := newScenarioRun(
		ScenarioOutbound,
		fmt.Sprintf("正在通过真实出库 Service 执行 %d 件出库流程", qty),
		ScenarioStep{Title: "创建出库单", Detail: fmt.Sprintf("创建 %d 件出库草稿", qty)},
		ScenarioStep{Title: "提交出库单", Detail: "将出库单从草稿推进到已提交状态"},
		ScenarioStep{Title: "审核并分配库存", Detail: "按入库时间 FIFO 分配库存并生成拣货任务"},
		ScenarioStep{Title: "完成拣货", Detail: "按真实拣货任务扣减库存"},
	)

	var orderID int64
	var orderNo string
	if err := run.execute(0, "DRAFT", "outbound.Service.Create", func() (string, error) {
		order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			BizOrderNo:  demoBizOrderNo(1),
			Remark:      "一键演示：出库审核、FIFO 分配、拣货出库",
			Details: []outbounddto.OrderDetailItem{{
				SKUID: refs.SKU.ID, ExpectedQty: qty,
			}},
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
	if err := run.execute(1, "DRAFT → SUBMITTED", "outbound.Service.Submit", func() (string, error) {
		if err := s.outbound.Submit(ctx, orderID); err != nil {
			return "", err
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}
	if err := run.execute(2, "SUBMITTED → PICKING", "outbound.Service.Approve", func() (string, error) {
		if err := s.outbound.Approve(ctx, orderID, operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s / FIFO 分配 %d 件", orderNo, qty), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(3, "PICKING → SHIPPED", "outbound.Service.Pick", func() (string, error) {
		detail, err := s.outbound.Get(ctx, orderID)
		if err != nil {
			return "", err
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
				return "", err
			}
			picked += pickQty
		}
		if picked != qty {
			return "", errcode.DemoDataMissing
		}
		return fmt.Sprintf("%s / %s / %d 件", orderNo, refs.SKU.Code, picked), nil
	}); err != nil {
		return run.result, err
	}

	return run.finish(fmt.Sprintf("出库单 %s 已完成 FIFO 分配并拣货 %d 件", orderNo, qty)), nil
}
