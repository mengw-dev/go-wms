package service

import (
	"context"
	"fmt"

	inbounddto "gowms/internal/modules/inbound/dto"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runInboundDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const qty = 10
	operator := s.Username(ctx)
	order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		Remark:      "一键演示：入库、收货、上架完整流程",
		Details: []inbounddto.OrderDetailItem{{
			SKUID: refs.SKU.ID, ExpectedQty: qty,
		}},
	}, operator)
	if err != nil {
		return nil, err
	}
	if err := s.inbound.Submit(ctx, order.ID); err != nil {
		return nil, err
	}
	if err := s.inbound.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	detail, err := s.inbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	if len(detail.Details) == 0 {
		return nil, errcode.DemoDataMissing
	}
	batchNo := demoBatchNo()
	if err := s.inbound.Receive(ctx, order.ID, detail.Details[0].ID, &inbounddto.ReceiveReq{
		DetailID: detail.Details[0].ID, Qty: qty, BatchNo: batchNo,
	}, operator); err != nil {
		return nil, err
	}

	detail, err = s.inbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	var putawayTaskID int64
	var putawayQty int
	for _, t := range detail.Tasks {
		if t.TaskType == taskmodel.TaskPutaway && t.Status != taskmodel.TaskCompleted && t.DoneQty < t.TargetQty {
			putawayTaskID = t.ID
			putawayQty = t.TargetQty - t.DoneQty
			break
		}
	}
	if putawayTaskID == 0 {
		return nil, errcode.DemoDataMissing
	}
	if err := s.inbound.Putaway(ctx, putawayTaskID, refs.Location.ID, putawayQty, operator); err != nil {
		return nil, err
	}

	return &ScenarioResult{
		Name:    ScenarioInbound,
		Summary: fmt.Sprintf("入库单 %s 已完成收货并上架 %d 件", order.OrderNo, qty),
		Steps: []ScenarioStep{
			{Title: "创建入库单", Detail: order.OrderNo},
			{Title: "提交并审核", Detail: "生成收货任务，状态机推进到 APPROVED"},
			{Title: "完成收货", Detail: fmt.Sprintf("货品 %s，批次 %s，数量 %d", refs.SKU.Code, batchNo, qty)},
			{Title: "完成上架", Detail: fmt.Sprintf("库位 %s，库存已增加", refs.Location.Code)},
		},
	}, nil
}
