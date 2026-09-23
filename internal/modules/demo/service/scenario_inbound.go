package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gorm.io/gorm"

	inbounddto "gowms/internal/modules/inbound/dto"
	inventorymodel "gowms/internal/modules/inventory/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runInboundDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const qty = 10
	operator := s.Username(ctx)
	run := newScenarioRun(
		ScenarioInbound,
		fmt.Sprintf("正在通过真实入库 Service 执行 %d 件入库流程", qty),
		ScenarioStep{Title: "创建入库单", Detail: fmt.Sprintf("创建 %s × %d 件入库草稿", refs.SKU.Code, qty)},
		ScenarioStep{Title: "提交入库单", Detail: "将入库单从草稿推进到已提交状态"},
		ScenarioStep{Title: "审核入库单", Detail: "审核通过并生成收货任务"},
		ScenarioStep{Title: "完成收货", Detail: "登记真实批次和收货数量"},
		ScenarioStep{Title: "完成上架", Detail: fmt.Sprintf("上架到目标库位 %s", refs.Location.Code)},
		ScenarioStep{Title: "库存入账", Detail: "核对上架事务写入的 RECEIVE 流水和三数量变化"},
	)

	var orderID int64
	var orderNo string
	var taskNumbers []string
	var putawayTaskNo string
	var putawayQty int
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
	run.result.Steps[3].Detail = fmt.Sprintf("登记批次 %s，收货 %d 件", batchNo, qty)
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
		taskNumbers = taskNumbers[:0]
		for _, task := range detail.Tasks {
			taskNumbers = append(taskNumbers, task.TaskNo)
		}
		var putawayTaskID int64
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

	var trans inventorymodel.InventoryTrans
	if err := run.execute(5, "", "inventory.Service.Increase → repository.InsertTrans", func() (string, error) {
		err := s.db.WithContext(ctx).
			Where("order_no = ? AND task_no = ? AND trans_type = ?", orderNo, putawayTaskNo, inventorymodel.TransReceive).
			Order("created_at DESC").
			First(&trans).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errcode.DemoDataMissing
		}
		if err != nil {
			return "", err
		}
		return inventoryStockChangeSummary(&trans), nil
	}); err != nil {
		return run.result, err
	}

	steps := run.result.Steps
	steps[0].Facts = []ScenarioFact{
		{Label: "入库单号", Value: orderNo},
		{Label: "仓库", Value: fmt.Sprintf("%s / %s", refs.Warehouse.Code, refs.Warehouse.Name)},
		{Label: "SKU", Value: fmt.Sprintf("%s / %s", refs.SKU.Code, refs.SKU.Name)},
		{Label: "入库数量", Value: fmt.Sprintf("%d 件", qty)},
	}
	steps[1].Facts = []ScenarioFact{{Label: "单据状态", Value: "DRAFT → SUBMITTED"}}
	steps[2].Facts = []ScenarioFact{{Label: "单据状态", Value: "SUBMITTED → APPROVED"}, {Label: "后续作业", Value: "生成收货任务"}}
	steps[3].Facts = []ScenarioFact{{Label: "批次", Value: batchNo}, {Label: "收货数量", Value: fmt.Sprintf("%d 件", qty)}}
	steps[4].Facts = []ScenarioFact{
		{Label: "上架任务", Value: putawayTaskNo},
		{Label: "库位", Value: refs.Location.Code},
		{Label: "上架数量", Value: fmt.Sprintf("%d 件", putawayQty)},
	}
	steps[5].Facts = stockChangeFacts(&trans)

	result := run.finish(fmt.Sprintf("入库单 %s 已完成 %d 件收货、上架和库存入账", orderNo, qty))
	result.EvidenceTitle = "本次入库产生"
	result.Evidence = []ScenarioEvidence{
		{
			Label:  "入库单",
			Value:  orderNo,
			Detail: fmt.Sprintf("%s / %s × %d", refs.Warehouse.Code, refs.SKU.Code, qty),
		},
		{
			Label:  "作业任务",
			Value:  fmt.Sprintf("%d 个", len(taskNumbers)),
			Detail: strings.Join(taskNumbers, "、"),
		},
		{
			Label:  "库存流水",
			Value:  fmt.Sprintf("%+d", trans.QuantityChange),
			Detail: fmt.Sprintf("%s / %s", trans.TransType, trans.TaskNo),
		},
		{
			Label:  "库存变化",
			Value:  inventoryStockChangeSummary(&trans),
			Detail: fmt.Sprintf("批次 %s / 库位 %s", batchNo, refs.Location.Code),
		},
	}
	result.Links = []ScenarioLink{
		{Label: "查看入库单", Path: fmt.Sprintf("/inbound/orders/%d", orderID)},
		{Label: "查看任务", Path: fmt.Sprintf("/tasks?order_id=%d", orderID)},
		{Label: "查看库存", Path: "/inventory?sku_keyword=" + url.QueryEscape(refs.SKU.Code)},
		{Label: "查看库存流水", Path: "/inventory?order_no=" + url.QueryEscape(orderNo)},
	}
	result.Implementation = &ScenarioImplementation{
		Orchestration: "internal/modules/demo/service/scenario_inbound.go",
		BusinessFiles: []string{
			"internal/modules/inbound/service/order.go",
			"internal/modules/inbound/service/receiving.go",
			"internal/modules/inbound/service/putaway.go",
			"internal/modules/task/service/service.go",
			"internal/modules/inventory/service/stock.go",
		},
		CallChain: []string{"Demo Orchestrator", "Inbound Service", "Task Service", "Inventory Service", "MySQL"},
	}
	return result, nil
}

func inventoryStockChangeSummary(trans *inventorymodel.InventoryTrans) string {
	if trans == nil {
		return ""
	}
	allocatedBefore := trans.BeforeQuantity - trans.AvailableBefore
	allocatedAfter := trans.AfterQuantity - trans.AvailableAfter
	return fmt.Sprintf(
		"库存 %+d / 现存量 %d → %d / 可用量 %d → %d / 已分配 %d → %d",
		trans.QuantityChange, trans.BeforeQuantity, trans.AfterQuantity,
		trans.AvailableBefore, trans.AvailableAfter, allocatedBefore, allocatedAfter,
	)
}

func stockChangeFacts(trans *inventorymodel.InventoryTrans) []ScenarioFact {
	if trans == nil {
		return nil
	}
	allocatedBefore := trans.BeforeQuantity - trans.AvailableBefore
	allocatedAfter := trans.AfterQuantity - trans.AvailableAfter
	return []ScenarioFact{
		{Label: "库存变化", Value: fmt.Sprintf("%+d", trans.QuantityChange)},
		{Label: "现存量", Value: fmt.Sprintf("%d → %d", trans.BeforeQuantity, trans.AfterQuantity)},
		{Label: "可用量", Value: fmt.Sprintf("%d → %d", trans.AvailableBefore, trans.AvailableAfter)},
		{Label: "已分配", Value: fmt.Sprintf("%d → %d", allocatedBefore, allocatedAfter)},
		{Label: "流水类型", Value: string(trans.TransType)},
		{Label: "流水任务", Value: trans.TaskNo},
	}
}
