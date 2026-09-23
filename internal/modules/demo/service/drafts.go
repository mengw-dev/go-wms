package service

import (
	"context"
	"fmt"

	inbounddto "gowms/internal/modules/inbound/dto"
	outbounddto "gowms/internal/modules/outbound/dto"
	stocktakedto "gowms/internal/modules/stocktake/dto"
)

// createInboundDrafts 模拟 Excel 批量导入：只创建草稿，不自动提交和收货。
func (s *Service) createInboundDrafts(ctx context.Context, refs *demoRefs, count, qty int) (*ScenarioResult, error) {
	count, qty = normalizeDraftOptions(count, qty, 3, 20)
	run := newScenarioRun(
		ScenarioInboundDrafts,
		fmt.Sprintf("正在通过真实入库 Service 创建 %d 张入库草稿", count),
		ScenarioStep{Title: "批量创建入库单", Detail: fmt.Sprintf("创建 %d 张草稿，每张 %d 件", count, qty)},
		ScenarioStep{Title: "下一步", Detail: "在入库单页面依次执行批量提交、审核、收货和上架"},
	)
	orderNos := make([]string, 0, count)
	if err := run.execute(0, "DRAFT", "inbound.Service.Create", func() (string, error) {
		for i := 0; i < count; i++ {
			order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
				WarehouseID: refs.Warehouse.ID,
				Remark:      fmt.Sprintf("模拟 Excel 批量导入：第 %d 批，每单 %d 件", i+1, qty),
				Details: []inbounddto.OrderDetailItem{{
					SKUID: refs.SKU.ID, ExpectedQty: qty,
				}},
			}, s.Username(ctx))
			if err != nil {
				return "", err
			}
			orderNos = append(orderNos, order.OrderNo)
		}
		return fmt.Sprintf("%d 张入库单", len(orderNos)), nil
	}); err != nil {
		return run.result, err
	}
	result := run.finish(fmt.Sprintf("已创建 %d 张入库草稿（每张 %d 件），请在入库单页面批量提交、审核、收货和上架", len(orderNos), qty))
	result.TargetPath = "/inbound/orders"
	result.TargetLabel = "前往入库单处理"
	return result, nil
}

// createOutboundDrafts 模拟上游 OMS/ERP 批量推送：只创建草稿，不自动分配和拣货。
func (s *Service) createOutboundDrafts(ctx context.Context, refs *demoRefs, count, qty int) (*ScenarioResult, error) {
	count, qty = normalizeDraftOptions(count, qty, 3, 5)
	run := newScenarioRun(
		ScenarioOutboundDrafts,
		fmt.Sprintf("正在通过真实出库 Service 创建 %d 张出库草稿", count),
		ScenarioStep{Title: "模拟上游批量导入", Detail: fmt.Sprintf("创建 %d 张草稿，每张 %d 件", count, qty)},
		ScenarioStep{Title: "下一步", Detail: "在出库单页面提交并审核，审核时执行 FIFO 分配并生成拣货任务"},
	)
	orderNos := make([]string, 0, count)
	if err := run.execute(0, "DRAFT", "outbound.Service.Create", func() (string, error) {
		for i := 0; i < count; i++ {
			order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
				WarehouseID: refs.Warehouse.ID,
				BizOrderNo:  demoBizOrderNo(i + 1),
				Remark:      fmt.Sprintf("模拟上游系统批量导入：第 %d 单，每单 %d 件", i+1, qty),
				Details: []outbounddto.OrderDetailItem{{
					SKUID: refs.SKU.ID, ExpectedQty: qty,
				}},
			}, s.Username(ctx))
			if err != nil {
				return "", err
			}
			orderNos = append(orderNos, order.OrderNo)
		}
		return fmt.Sprintf("%d 张出库单", len(orderNos)), nil
	}); err != nil {
		return run.result, err
	}
	result := run.finish(fmt.Sprintf("已模拟上游系统创建 %d 张出库草稿（每张 %d 件），请在出库单页面批量提交、审核和拣货", len(orderNos), qty))
	result.TargetPath = "/outbound/orders"
	result.TargetLabel = "前往出库单处理"
	return result, nil
}

// createStocktakeDrafts 批量创建盘点草稿，实际盘点数量和审核由体验者自己完成。
func (s *Service) createStocktakeDrafts(ctx context.Context, refs *demoRefs, count int) (*ScenarioResult, error) {
	count, _ = normalizeDraftOptions(count, 0, 2, 0)
	run := newScenarioRun(
		ScenarioStocktakeDrafts,
		fmt.Sprintf("正在通过真实盘点 Service 创建 %d 张盘点草稿", count),
		ScenarioStep{Title: "批量创建盘点单", Detail: fmt.Sprintf("创建 %d 张盘点单并生成库存快照", count)},
		ScenarioStep{Title: "下一步", Detail: "进入盘点详情录入实盘数量，再执行审核和库存调整"},
	)
	orderNos := make([]string, 0, count)
	if err := run.execute(0, "CREATED", "stocktake.Service.Create", func() (string, error) {
		for i := 0; i < count; i++ {
			order, err := s.stocktake.Create(ctx, &stocktakedto.CreateOrderReq{
				WarehouseID: refs.Warehouse.ID,
				Remark:      fmt.Sprintf("模拟盘点任务：第 %d 批", i+1),
			}, s.Username(ctx))
			if err != nil {
				return "", err
			}
			orderNos = append(orderNos, order.OrderNo)
		}
		return fmt.Sprintf("%d 张盘点单", len(orderNos)), nil
	}); err != nil {
		return run.result, err
	}
	result := run.finish(fmt.Sprintf("已创建 %d 张盘点草稿，请在盘点单页面录入实盘数量并审核", len(orderNos)))
	result.TargetPath = "/stocktake/orders"
	result.TargetLabel = "前往盘点单处理"
	return result, nil
}
func normalizeDraftOptions(count, qty, defaultCount, defaultQty int) (int, int) {
	if count <= 0 {
		count = defaultCount
	}
	if count > 20 {
		count = 20
	}
	if defaultQty > 0 {
		if qty <= 0 {
			qty = defaultQty
		}
		if qty > 1000 {
			qty = 1000
		}
	}
	return count, qty
}
