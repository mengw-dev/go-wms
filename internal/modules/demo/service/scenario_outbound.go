package service

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	inventorymodel "gowms/internal/modules/inventory/model"
	outbounddto "gowms/internal/modules/outbound/dto"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runOutboundDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const qty = 50
	operator := s.Username(ctx)
	run := newScenarioRun(
		ScenarioOutbound,
		fmt.Sprintf("正在通过真实出库 Service 执行 %d 件出库流程", qty),
		ScenarioStep{Title: "创建出库单", Detail: fmt.Sprintf("创建 %s × %d 件出库草稿", refs.SKU.Code, qty)},
		ScenarioStep{Title: "提交出库单", Detail: "将出库单从草稿推进到已提交状态"},
		ScenarioStep{Title: "审核并 FIFO 分配库存", Detail: "按库存入库时间分配批次，并生成拣货任务"},
		ScenarioStep{Title: "完成拣货与库存扣减", Detail: "执行真实 PICK 任务并核对 SHIP 库存流水"},
	)

	var orderID int64
	var orderNo string
	var allocations []*outboundmodel.Allocation
	var inventories map[int64]*inventorymodel.Inventory
	var pickTasks []*taskmodel.Task
	var allocatedTotal int
	var trans []*inventorymodel.InventoryTrans

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
		run.result.Steps[0].Facts = []ScenarioFact{
			{Label: "出库单号", Value: orderNo},
			{Label: "仓库", Value: fmt.Sprintf("%s / %s", refs.Warehouse.Code, refs.Warehouse.Name)},
			{Label: "SKU", Value: fmt.Sprintf("%s / %s", refs.SKU.Code, refs.SKU.Name)},
			{Label: "出库需求", Value: fmt.Sprintf("%d 件", qty)},
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(1, "DRAFT → SUBMITTED", "outbound.Service.Submit", func() (string, error) {
		if err := s.outbound.Submit(ctx, orderID); err != nil {
			return "", err
		}
		run.result.Steps[1].Facts = []ScenarioFact{{Label: "单据状态", Value: "DRAFT → SUBMITTED"}}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(2, "SUBMITTED → PICKING", "outbound.Service.Approve", func() (string, error) {
		if err := s.outbound.Approve(ctx, orderID, operator); err != nil {
			return "", err
		}
		detail, err := s.outbound.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		allocations = detail.Allocations
		allocatedTotal = 0
		for _, allocation := range allocations {
			allocatedTotal += allocation.AllocatedQty
		}
		if allocatedTotal != qty || len(allocations) == 0 {
			return "", errcode.DemoDataMissing
		}
		inventories, err = s.loadOutboundAllocationInventories(ctx, allocations)
		if err != nil {
			return "", err
		}

		pickTasks = pickTasks[:0]
		for _, task := range detail.Tasks {
			if task.TaskType == taskmodel.TaskPick {
				pickTasks = append(pickTasks, task)
			}
		}
		if len(pickTasks) == 0 {
			return "", errcode.DemoDataMissing
		}

		run.result.Steps[2].Facts = append(
			outboundFIFOFacts(allocations, inventories, qty),
			ScenarioFact{Label: "PICK 任务数量", Value: fmt.Sprintf("%d 个", len(pickTasks))},
		)
		return fmt.Sprintf("%s / FIFO %d 件 / PICK %d 个", orderNo, allocatedTotal, len(pickTasks)), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(3, "PICKING → SHIPPED", "outbound.Service.Pick → inventory.Service.Ship", func() (string, error) {
		var picked int
		for _, task := range pickTasks {
			pickQty := task.TargetQty - task.DoneQty
			if pickQty <= 0 {
				continue
			}
			if err := s.outbound.Pick(ctx, task.ID, pickQty, operator, nil); err != nil {
				return "", err
			}
			picked += pickQty
		}
		if picked != qty {
			return "", errcode.DemoDataMissing
		}

		detail, err := s.outbound.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		if detail.Order.Status != outboundmodel.OrderShipped {
			return "", errcode.DemoDataMissing
		}
		loadedTrans, err := s.loadOutboundInventoryTrans(ctx, orderNo)
		if err != nil {
			return "", err
		}
		trans = loadedTrans
		summary := summarizeOutboundTrans(trans)
		if summary.AllocateCount != len(allocations) ||
			summary.ShipCount != len(allocations) ||
			summary.StockChange != -qty {
			return "", errcode.DemoDataMissing
		}

		run.result.Steps[3].Facts = append(
			outboundPickTaskFacts(detail.Tasks),
			inventoryTransFacts(trans)...,
		)
		return fmt.Sprintf("%s / %d 个 PICK 任务 / 库存 %+d", orderNo, summary.ShipCount, summary.StockChange), nil
	}); err != nil {
		return run.result, err
	}

	result := run.finish(fmt.Sprintf("出库单 %s 已按 FIFO 分配并拣货 %d 件", orderNo, qty))
	result.EvidenceTitle = "本次出库产生"
	result.Evidence = []ScenarioEvidence{
		{
			Label:  "出库单",
			Value:  orderNo,
			Detail: fmt.Sprintf("%s / %s × %d", refs.Warehouse.Code, refs.SKU.Code, qty),
		},
		{
			Label:  "FIFO 分配",
			Value:  fmt.Sprintf("%d 个批次 / %d 件", len(allocations), allocatedTotal),
			Detail: outboundFIFOEvidence(allocations, inventories),
		},
		{
			Label:  "PICK 任务",
			Value:  fmt.Sprintf("%d 个", len(pickTasks)),
			Detail: strings.Join(outboundTaskNumbers(pickTasks), "、"),
		},
		{
			Label:  "库存流水",
			Value:  fmt.Sprintf("%d 条", len(trans)),
			Detail: outboundTransEvidence(trans),
		},
		{
			Label:  "库存变化",
			Value:  outboundStockChangeValue(trans),
			Detail: outboundStockChangeDetail(trans),
		},
	}
	result.Links = []ScenarioLink{
		{Label: "查看出库单", Path: fmt.Sprintf("/outbound/orders/%d", orderID)},
		{Label: "查看拣货任务", Path: fmt.Sprintf("/tasks?order_id=%d", orderID)},
		{Label: "查看库存流水", Path: "/inventory?order_no=" + url.QueryEscape(orderNo)},
	}
	result.Implementation = &ScenarioImplementation{
		Orchestration: "internal/modules/demo/service/scenario_outbound.go",
		BusinessFiles: []string{
			"internal/modules/outbound/service/order.go",
			"internal/modules/outbound/service/pick.go",
			"internal/modules/inventory/service/stock.go",
			"internal/modules/task/service/service.go",
		},
		CallChain: []string{"Demo Orchestrator", "Outbound Service", "Inventory / Task", "MySQL"},
	}
	return result, nil
}

func (s *Service) loadOutboundAllocationInventories(ctx context.Context, allocations []*outboundmodel.Allocation) (map[int64]*inventorymodel.Inventory, error) {
	ids := make([]int64, 0, len(allocations))
	seen := make(map[int64]struct{}, len(allocations))
	for _, allocation := range allocations {
		if _, ok := seen[allocation.InventoryID]; ok {
			continue
		}
		seen[allocation.InventoryID] = struct{}{}
		ids = append(ids, allocation.InventoryID)
	}
	if len(ids) == 0 {
		return nil, errcode.DemoDataMissing
	}
	var rows []*inventorymodel.Inventory
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	inventories := make(map[int64]*inventorymodel.Inventory, len(rows))
	for _, inventory := range rows {
		inventories[inventory.ID] = inventory
	}
	if len(inventories) != len(ids) {
		return nil, errcode.DemoDataMissing
	}
	return inventories, nil
}

func (s *Service) loadOutboundInventoryTrans(ctx context.Context, orderNo string) ([]*inventorymodel.InventoryTrans, error) {
	var trans []*inventorymodel.InventoryTrans
	err := s.db.WithContext(ctx).
		Where("order_no = ? AND trans_type IN ?", orderNo,
			[]inventorymodel.TransType{inventorymodel.TransAllocate, inventorymodel.TransShip}).
		Order("id").
		Find(&trans).Error
	if err != nil {
		return nil, err
	}
	if len(trans) == 0 {
		return nil, errcode.DemoDataMissing
	}
	return trans, nil
}

func sortOutboundAllocationsByFIFO(allocations []*outboundmodel.Allocation, inventories map[int64]*inventorymodel.Inventory) []*outboundmodel.Allocation {
	sorted := append([]*outboundmodel.Allocation(nil), allocations...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := inventories[sorted[i].InventoryID]
		right := inventories[sorted[j].InventoryID]
		if left != nil && right != nil && !left.StockInTime.Equal(right.StockInTime) {
			return left.StockInTime.Before(right.StockInTime)
		}
		if sorted[i].LocationCode != sorted[j].LocationCode {
			return sorted[i].LocationCode < sorted[j].LocationCode
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

func outboundFIFOFacts(allocations []*outboundmodel.Allocation, inventories map[int64]*inventorymodel.Inventory, demand int) []ScenarioFact {
	sorted := sortOutboundAllocationsByFIFO(allocations, inventories)
	allocated := 0
	for _, allocation := range sorted {
		allocated += allocation.AllocatedQty
	}
	facts := []ScenarioFact{{
		Label: "分配汇总",
		Value: fmt.Sprintf("需求 %d / 已分配 %d / %d 个批次", demand, allocated, len(sorted)),
	}}
	for index, allocation := range sorted {
		inventory := inventories[allocation.InventoryID]
		stockInAt := "时间未知"
		available := 0
		if inventory != nil {
			stockInAt = formatOutboundStockIn(inventory.StockInTime)
			available = inventory.AvailableQty
		}
		rawBatchNo := "-"
		if strings.TrimSpace(allocation.BatchNo) != "" {
			rawBatchNo = allocation.BatchNo
		}
		facts = append(facts, ScenarioFact{
			Label: fmt.Sprintf("FIFO %d", index+1),
			Value: fmt.Sprintf("批次 %s / 入库 %s / 分配后可用 %d / 本次分配 %d / 库位 %s",
				rawBatchNo, stockInAt, available, allocation.AllocatedQty, allocation.LocationCode),
		})
	}
	return facts
}

func outboundPickTaskFacts(tasks []*taskmodel.Task) []ScenarioFact {
	facts := make([]ScenarioFact, 0, len(tasks)+1)
	pickCount := 0
	for _, task := range tasks {
		if task.TaskType != taskmodel.TaskPick {
			continue
		}
		pickCount++
		batchNo := task.BatchNo
		if strings.TrimSpace(batchNo) == "" {
			batchNo = "-"
		}
		facts = append(facts, ScenarioFact{
			Label: fmt.Sprintf("PICK %d", pickCount),
			Value: fmt.Sprintf("%s / %s / 批次 %s / 库位 %s / 完成 %d/%d",
				task.TaskNo, task.Status, batchNo, task.LocationCode, task.DoneQty, task.TargetQty),
		})
	}
	return append([]ScenarioFact{{Label: "PICK 任务数量", Value: fmt.Sprintf("%d 个", pickCount)}}, facts...)
}

func inventoryTransFacts(trans []*inventorymodel.InventoryTrans) []ScenarioFact {
	facts := make([]ScenarioFact, 0, len(trans))
	for index, item := range trans {
		allocatedBefore := item.BeforeQuantity - item.AvailableBefore
		allocatedAfter := item.AfterQuantity - item.AvailableAfter
		facts = append(facts, ScenarioFact{
			Label: fmt.Sprintf("流水 %d", index+1),
			Value: fmt.Sprintf("%s / 数量 %+d / 现有 %d → %d / 可用 %d → %d / 已分配 %d → %d / 任务 %s",
				item.TransType, item.QuantityChange, item.BeforeQuantity, item.AfterQuantity,
				item.AvailableBefore, item.AvailableAfter, allocatedBefore, allocatedAfter, item.TaskNo),
		})
	}
	return facts
}

type outboundStockSummary struct {
	AllocateCount   int
	ShipCount       int
	StockChange     int
	StockBefore     int
	StockAfter      int
	AvailableBefore int
	AvailableAfter  int
	AllocatedBefore int
	AllocatedAfter  int
}

func summarizeOutboundTrans(trans []*inventorymodel.InventoryTrans) outboundStockSummary {
	var summary outboundStockSummary
	for _, item := range trans {
		switch item.TransType {
		case inventorymodel.TransAllocate:
			summary.AllocateCount++
			summary.StockBefore += item.BeforeQuantity
			summary.AvailableBefore += item.AvailableBefore
			summary.AllocatedBefore += item.BeforeQuantity - item.AvailableBefore
		case inventorymodel.TransShip:
			summary.ShipCount++
			summary.StockChange += item.QuantityChange
			summary.StockAfter += item.AfterQuantity
			summary.AvailableAfter += item.AvailableAfter
			summary.AllocatedAfter += item.AfterQuantity - item.AvailableAfter
		}
	}
	return summary
}

func outboundFIFOEvidence(allocations []*outboundmodel.Allocation, inventories map[int64]*inventorymodel.Inventory) string {
	sorted := sortOutboundAllocationsByFIFO(allocations, inventories)
	parts := make([]string, 0, len(sorted))
	for _, allocation := range sorted {
		batchNo := allocation.BatchNo
		if strings.TrimSpace(batchNo) == "" {
			batchNo = "-"
		}
		parts = append(parts, fmt.Sprintf("%s %d 件@%s", batchNo, allocation.AllocatedQty, allocation.LocationCode))
	}
	return strings.Join(parts, "、")
}

func outboundTaskNumbers(tasks []*taskmodel.Task) []string {
	numbers := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if task.TaskType == taskmodel.TaskPick {
			numbers = append(numbers, task.TaskNo)
		}
	}
	return numbers
}

func outboundTransEvidence(trans []*inventorymodel.InventoryTrans) string {
	summary := summarizeOutboundTrans(trans)
	return fmt.Sprintf("ALLOCATE %d 条 / SHIP %d 条", summary.AllocateCount, summary.ShipCount)
}

func outboundStockChangeValue(trans []*inventorymodel.InventoryTrans) string {
	summary := summarizeOutboundTrans(trans)
	return fmt.Sprintf("%+d", summary.StockChange)
}

func outboundStockChangeDetail(trans []*inventorymodel.InventoryTrans) string {
	summary := summarizeOutboundTrans(trans)
	return fmt.Sprintf("现有 %d → %d / 可用 %d → %d / 已分配 %d → %d",
		summary.StockBefore, summary.StockAfter, summary.AvailableBefore, summary.AvailableAfter,
		summary.AllocatedBefore, summary.AllocatedAfter)
}

func formatOutboundStockIn(stockIn time.Time) string {
	if stockIn.IsZero() {
		return "时间未知"
	}
	return stockIn.Format("2006-01-02 15:04")
}
