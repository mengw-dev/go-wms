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
	run := newScenarioRun(
		ScenarioInbound,
		fmt.Sprintf("正在通过真实入库 Service 执行 %d 件入库流程", qty),
		ScenarioStep{Title: "创建入库单", Detail: fmt.Sprintf("创建 %d 件入库草稿", qty)},
		ScenarioStep{Title: "提交入库单", Detail: "将入库单从草稿推进到已提交状态"},
		ScenarioStep{Title: "审核入库单", Detail: "审核通过并生成收货任务"},
		ScenarioStep{Title: "完成收货", Detail: "登记真实批次和收货数量"},
		ScenarioStep{Title: "完成上架", Detail: "上架到目标库位并增加库存"},
	)

	var orderID int64
	var orderNo string
	if err := run.execute(0, "DRAFT", "inbound.Service.Create", func() (string, error) {
		order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			Remark:      "一键演示：入库、收货、上架完整流程",
			Details: []inbounddto.OrderDetailItem{{
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
	if err := run.execute(1, "DRAFT → SUBMITTED", "inbound.Service.Submit", func() (string, error) {
		if err := s.inbound.Submit(ctx, orderID); err != nil {
			return "", err
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}
	if err := run.execute(2, "SUBMITTED → APPROVED", "inbound.Service.Approve", func() (string, error) {
		if err := s.inbound.Approve(ctx, orderID, operator); err != nil {
			return "", err
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	batchNo := demoBatchNo()
	if err := run.execute(3, "APPROVED → PUTAWAY", "inbound.Service.Receive", func() (string, error) {
		detail, err := s.inbound.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		if len(detail.Details) == 0 {
			return "", errcode.DemoDataMissing
		}
		if err := s.inbound.Receive(ctx, orderID, detail.Details[0].ID, &inbounddto.ReceiveReq{
			DetailID: detail.Details[0].ID, Qty: qty, BatchNo: batchNo,
		}, operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s / 批次 %s / %d 件", orderNo, batchNo, qty), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(4, "PUTAWAY → COMPLETED", "inbound.Service.Putaway", func() (string, error) {
		detail, err := s.inbound.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		var putawayTaskID int64
		var putawayTaskNo string
		var putawayQty int
		for _, t := range detail.Tasks {
			if t.TaskType == taskmodel.TaskPutaway && t.Status != taskmodel.TaskCompleted && t.DoneQty < t.TargetQty {
				putawayTaskID = t.ID
				putawayTaskNo = t.TaskNo
				putawayQty = t.TargetQty - t.DoneQty
				break
			}
		}
		if putawayTaskID == 0 {
			return "", errcode.DemoDataMissing
		}
		if err := s.inbound.Putaway(ctx, putawayTaskID, refs.Location.ID, putawayQty, operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s / 库位 %s / %d 件", putawayTaskNo, refs.Location.Code, putawayQty), nil
	}); err != nil {
		return run.result, err
	}

	return run.finish(fmt.Sprintf("入库单 %s 已完成收货并上架 %d 件", orderNo, qty)), nil
}
