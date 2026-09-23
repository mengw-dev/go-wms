package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	inventorymodel "gowms/internal/modules/inventory/model"
	stocktakedto "gowms/internal/modules/stocktake/dto"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) runStocktakeDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const varianceQty = 3

	operator := s.Username(ctx)
	run := newScenarioRun(
		ScenarioStocktake,
		"正在通过真实盘点 Service 执行库存快照、实盘、差异审核和库存调整",
		ScenarioStep{Title: "创建盘点", Detail: "为演示仓库创建整仓盘点单"},
		ScenarioStep{Title: "库存快照", Detail: "读取创建盘点时生成的库存账面快照"},
		ScenarioStep{Title: "录入实盘", Detail: "录入真实实盘数量，并包含一笔可核对的盘亏"},
		ScenarioStep{Title: "核对差异", Detail: "按账面与实盘结果计算盘点差异"},
		ScenarioStep{Title: "审核盘点", Detail: "审核真实盘点单并触发库存调整"},
		ScenarioStep{Title: "库存调整入账", Detail: "核对审核事务写入的 ADJUST 流水和三数量变化"},
	)

	var orderID int64
	var orderNo string
	var details []*stocktakemodel.StocktakeDetail

	if err := run.execute(0, "DRAFT", "stocktake.Service.Create", func() (string, error) {
		order, err := s.stocktake.Create(ctx, &stocktakedto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			Remark:      "一键演示：库存快照、录入实盘、审核调整",
		}, operator)
		if err != nil {
			return "", err
		}
		detail, err := s.stocktake.Get(ctx, order.ID)
		if err != nil {
			return "", err
		}
		if len(detail.Details) == 0 {
			return "", errcode.DemoDataMissing
		}
		orderID = order.ID
		orderNo = order.OrderNo
		details = detail.Details
		run.result.Steps[0].Facts = []ScenarioFact{
			{Label: "盘点单号", Value: orderNo},
			{Label: "仓库", Value: fmt.Sprintf("%s / %s", refs.Warehouse.Code, refs.Warehouse.Name)},
			{Label: "盘点范围", Value: "整仓盘点"},
			{Label: "快照明细", Value: fmt.Sprintf("%d 条", len(details))},
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(1, "", "stocktake.Service.Get", func() (string, error) {
		detail, err := s.stocktake.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		details = detail.Details
		summary := summarizeStocktakeDetails(details, refs.SKU.ID)
		if summary.Lines == 0 {
			return "", errcode.DemoDataMissing
		}
		run.result.Steps[1].Facts = stocktakeSnapshotFacts(details, refs.SKU.ID)
		return fmt.Sprintf("%s / %s 账面 %d 件 / %d 条批次",
			orderNo, refs.SKU.Code, summary.BookQty, summary.Lines), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(2, "DRAFT / 实盘已录入", "stocktake.Service.RecordActual", func() (string, error) {
		detail, err := s.stocktake.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		details = detail.Details

		var target *stocktakemodel.StocktakeDetail
		for _, item := range details {
			if item.SKUID == refs.SKU.ID && item.BookQty >= varianceQty {
				target = item
				break
			}
		}
		if target == nil {
			return "", errcode.DemoDataMissing
		}

		for _, item := range details {
			actual := item.BookQty
			if item.ID == target.ID {
				actual -= varianceQty
			}
			if err := s.stocktake.RecordActual(ctx, orderID, item.ID, actual); err != nil {
				return "", err
			}
		}

		recorded, err := s.stocktake.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		details = recorded.Details
		summary := summarizeStocktakeDetails(details, refs.SKU.ID)
		if summary.Lines == 0 || summary.Counted != summary.Lines || summary.DiffQty != -varianceQty {
			return "", errcode.DemoDataMissing
		}
		run.result.Steps[2].Facts = stocktakeActualFacts(details, refs.SKU.ID)
		return fmt.Sprintf("%s / %d 条实盘明细 / 差异 %+d", orderNo, summary.Counted, summary.DiffQty), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(3, "DRAFT", "stocktake.Service.Get", func() (string, error) {
		detail, err := s.stocktake.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		details = detail.Details
		summary := summarizeStocktakeDetails(details, refs.SKU.ID)
		if summary.Lines == 0 || summary.Counted != summary.Lines || summary.DiffQty != -varianceQty {
			return "", errcode.DemoDataMissing
		}
		run.result.Steps[3].Facts = stocktakeDifferenceFacts(summary)
		return fmt.Sprintf("账面 %d / 实盘 %d / 差异 %+d",
			summary.BookQty, summary.ActualQty, summary.DiffQty), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(4, "DRAFT → COMPLETED", "stocktake.Service.Approve", func() (string, error) {
		if err := s.stocktake.Approve(ctx, orderID, operator); err != nil {
			return "", err
		}
		detail, err := s.stocktake.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		details = detail.Details
		if detail.Order.Status != stocktakemodel.OrderCompleted {
			return "", errcode.DemoDataMissing
		}
		summary := summarizeStocktakeDetails(details, refs.SKU.ID)
		if summary.Lines == 0 || summary.Counted != summary.Lines || summary.DiffQty != -varianceQty {
			return "", errcode.DemoDataMissing
		}
		for _, item := range details {
			if item.SKUID != refs.SKU.ID {
				continue
			}
			if item.ActualQty == nil || !item.Adjusted || item.DiffQty != *item.ActualQty-item.BookQty {
				return "", errcode.DemoDataMissing
			}
		}
		run.result.Steps[4].Facts = []ScenarioFact{
			{Label: "单据状态", Value: "DRAFT → COMPLETED"},
			{Label: "审核结果", Value: fmt.Sprintf("%d → %d", summary.BookQty, summary.ActualQty)},
			{Label: "确认差异", Value: fmt.Sprintf("%+d", summary.DiffQty)},
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	var adjustments []*inventorymodel.InventoryTrans
	if err := run.execute(5, "", "inventory.Service.Adjust → repository.InsertTrans", func() (string, error) {
		err := s.db.WithContext(ctx).
			Where("order_no = ? AND trans_type = ?", orderNo, inventorymodel.TransAdjust).
			Order("created_at DESC").
			Find(&adjustments).Error
		if err != nil {
			return "", err
		}
		if len(adjustments) != 1 {
			return "", errcode.DemoDataMissing
		}
		trans := adjustments[0]
		if trans.QuantityChange != -varianceQty ||
			trans.AfterQuantity-trans.BeforeQuantity != -varianceQty {
			return "", errcode.DemoDataMissing
		}
		run.result.Steps[5].Facts = stocktakeAdjustmentFacts(trans)
		return fmt.Sprintf("%s / %s %+d / %d → %d",
			orderNo, trans.TransType, trans.QuantityChange, trans.BeforeQuantity, trans.AfterQuantity), nil
	}); err != nil {
		return run.result, err
	}

	summary := summarizeStocktakeDetails(details, refs.SKU.ID)
	trans := adjustments[0]
	result := run.finish(fmt.Sprintf("盘点单 %s 已完成账面 %d → 实盘 %d，并写入 ADJUST %+d",
		orderNo, summary.BookQty, summary.ActualQty, trans.QuantityChange))
	result.EvidenceTitle = "本次盘点产生"
	result.Evidence = []ScenarioEvidence{
		{
			Label:  "盘点单",
			Value:  orderNo,
			Detail: fmt.Sprintf("%s / 整仓盘点", refs.Warehouse.Code),
		},
		{
			Label:  "库存快照",
			Value:  fmt.Sprintf("%d 条批次 / 账面 %d 件", summary.Lines, summary.BookQty),
			Detail: refs.SKU.Code,
		},
		{
			Label:  "实盘结果",
			Value:  fmt.Sprintf("账面 %d / 实盘 %d", summary.BookQty, summary.ActualQty),
			Detail: fmt.Sprintf("差异 %+d", summary.DiffQty),
		},
		{
			Label:  "库存流水",
			Value:  fmt.Sprintf("%s %+d", trans.TransType, trans.QuantityChange),
			Detail: fmt.Sprintf("盘点单 %s", trans.OrderNo),
		},
		{
			Label:  "库存调整",
			Value:  fmt.Sprintf("%d → %d", trans.BeforeQuantity, trans.AfterQuantity),
			Detail: fmt.Sprintf("可用量 %d → %d", trans.AvailableBefore, trans.AvailableAfter),
		},
	}
	result.Links = []ScenarioLink{
		{Label: "查看盘点单", Path: fmt.Sprintf("/stocktake/orders/%d", orderID)},
		{Label: "查看库存", Path: "/inventory?sku_keyword=" + url.QueryEscape(refs.SKU.Code)},
		{Label: "查看库存流水", Path: "/inventory?order_no=" + url.QueryEscape(orderNo)},
	}
	result.Implementation = &ScenarioImplementation{
		Orchestration: "internal/modules/demo/service/scenario_stocktake.go",
		BusinessFiles: []string{
			"internal/modules/stocktake/service/order.go",
			"internal/modules/stocktake/service/counting.go",
			"internal/modules/stocktake/service/approve.go",
			"internal/modules/inventory/service/stock.go",
		},
		CallChain: []string{"Demo Orchestrator", "Stocktake Service", "Inventory Service", "MySQL"},
	}
	return result, nil
}

type stocktakeSummary struct {
	BookQty   int
	ActualQty int
	DiffQty   int
	Lines     int
	Counted   int
}

func summarizeStocktakeDetails(details []*stocktakemodel.StocktakeDetail, skuID int64) stocktakeSummary {
	var summary stocktakeSummary
	for _, detail := range details {
		if detail == nil || detail.SKUID != skuID {
			continue
		}
		summary.Lines++
		summary.BookQty += detail.BookQty
		if detail.ActualQty == nil {
			continue
		}
		summary.Counted++
		summary.ActualQty += *detail.ActualQty
		summary.DiffQty += *detail.ActualQty - detail.BookQty
	}
	return summary
}

func stocktakeSnapshotFacts(details []*stocktakemodel.StocktakeDetail, skuID int64) []ScenarioFact {
	summary := summarizeStocktakeDetails(details, skuID)
	facts := []ScenarioFact{{
		Label: "快照汇总",
		Value: fmt.Sprintf("账面合计 %d 件 / %d 条批次", summary.BookQty, summary.Lines),
	}}
	line := 0
	for _, detail := range details {
		if detail == nil || detail.SKUID != skuID {
			continue
		}
		line++
		facts = append(facts, ScenarioFact{
			Label: fmt.Sprintf("快照 %d", line),
			Value: fmt.Sprintf("%s / 批次 %s / 库位 %s / 账面 %d",
				detail.SKUCode, stocktakeBatchNo(detail.BatchNo), detail.LocationCode, detail.BookQty),
		})
	}
	return facts
}

func stocktakeActualFacts(details []*stocktakemodel.StocktakeDetail, skuID int64) []ScenarioFact {
	summary := summarizeStocktakeDetails(details, skuID)
	facts := []ScenarioFact{{
		Label: "实盘明细",
		Value: fmt.Sprintf("%d 条", summary.Counted),
	}}
	line := 0
	for _, detail := range details {
		if detail == nil || detail.SKUID != skuID || detail.ActualQty == nil {
			continue
		}
		line++
		actual := *detail.ActualQty
		facts = append(facts, ScenarioFact{
			Label: fmt.Sprintf("实盘 %d", line),
			Value: fmt.Sprintf("%s / 批次 %s / 库位 %s / 账面 %d / 实盘 %d / 差异 %+d",
				detail.SKUCode, stocktakeBatchNo(detail.BatchNo), detail.LocationCode,
				detail.BookQty, actual, actual-detail.BookQty),
		})
	}
	return facts
}

func stocktakeDifferenceFacts(summary stocktakeSummary) []ScenarioFact {
	return []ScenarioFact{
		{Label: "账面数量", Value: fmt.Sprintf("%d 件", summary.BookQty)},
		{Label: "实盘数量", Value: fmt.Sprintf("%d 件", summary.ActualQty)},
		{Label: "盘点差异", Value: fmt.Sprintf("%+d", summary.DiffQty)},
	}
}

func stocktakeAdjustmentFacts(trans *inventorymodel.InventoryTrans) []ScenarioFact {
	if trans == nil {
		return nil
	}
	allocatedBefore := trans.BeforeQuantity - trans.AvailableBefore
	allocatedAfter := trans.AfterQuantity - trans.AvailableAfter
	return []ScenarioFact{
		{Label: "库存流水", Value: fmt.Sprintf("%s %+d", trans.TransType, trans.QuantityChange)},
		{Label: "现存量", Value: fmt.Sprintf("%d → %d", trans.BeforeQuantity, trans.AfterQuantity)},
		{Label: "可用量", Value: fmt.Sprintf("%d → %d", trans.AvailableBefore, trans.AvailableAfter)},
		{Label: "已分配", Value: fmt.Sprintf("%d → %d", allocatedBefore, allocatedAfter)},
		{Label: "流水单号", Value: trans.OrderNo},
	}
}

func stocktakeBatchNo(batchNo string) string {
	if strings.TrimSpace(batchNo) == "" {
		return "-"
	}
	return batchNo
}
