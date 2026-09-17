package service

import (
	"context"
	"fmt"
	"time"

	inbounddto "gowms/internal/modules/inbound/dto"
	outbounddto "gowms/internal/modules/outbound/dto"
	stocktakedto "gowms/internal/modules/stocktake/dto"
)

// createInboundDrafts 模拟 Excel 批量导入：只创建草稿，不自动提交和收货。
func (s *Service) createInboundDrafts(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	quantities := []int{20, 15, 10}
	orderNos := make([]string, 0, len(quantities))
	for i, qty := range quantities {
		order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			Remark:      fmt.Sprintf("模拟 Excel 批量导入：第 %d 批", i+1),
			Details: []inbounddto.OrderDetailItem{{
				SKUID: refs.SKU.ID, ExpectedQty: qty,
			}},
		}, s.Username())
		if err != nil {
			return nil, err
		}
		orderNos = append(orderNos, order.OrderNo)
	}
	return &ScenarioResult{
		Name:        ScenarioInboundDrafts,
		Summary:     fmt.Sprintf("已创建 %d 张入库草稿，请在入库单页面批量提交、审核、收货和上架", len(orderNos)),
		TargetPath:  "/inbound/orders",
		TargetLabel: "前往入库单处理",
		Steps: []ScenarioStep{
			{Title: "批量创建入库单", Detail: fmt.Sprintf("共 %d 张，状态均为草稿", len(orderNos))},
			{Title: "下一步", Detail: "HR 可在入库单页面勾选草稿，依次执行批量提交、批量审核、收货和上架"},
		},
	}, nil
}

// createOutboundDrafts 模拟上游 OMS/ERP 批量推送：只创建草稿，不自动分配和拣货。
func (s *Service) createOutboundDrafts(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	quantities := []int{5, 8, 10}
	orderNos := make([]string, 0, len(quantities))
	for i, qty := range quantities {
		order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			BizOrderNo:  fmt.Sprintf("UPSTREAM-DEMO-%d-%d", time.Now().UnixNano(), i+1),
			Remark:      fmt.Sprintf("模拟上游系统批量导入：第 %d 单", i+1),
			Details: []outbounddto.OrderDetailItem{{
				SKUID: refs.SKU.ID, ExpectedQty: qty,
			}},
		}, s.Username())
		if err != nil {
			return nil, err
		}
		orderNos = append(orderNos, order.OrderNo)
	}
	return &ScenarioResult{
		Name:        ScenarioOutboundDrafts,
		Summary:     fmt.Sprintf("已模拟上游系统创建 %d 张出库草稿，请在出库单页面批量提交、审核和拣货", len(orderNos)),
		TargetPath:  "/outbound/orders",
		TargetLabel: "前往出库单处理",
		Steps: []ScenarioStep{
			{Title: "模拟上游批量导入", Detail: fmt.Sprintf("共 %d 张出库单，状态均为草稿", len(orderNos))},
			{Title: "下一步", Detail: "HR 可在出库单页面批量提交、批量审核；审核时会执行 FIFO 分配并生成拣货任务"},
		},
	}, nil
}

// createStocktakeDrafts 批量创建盘点草稿，实际盘点数量和审核由 HR 自己完成。
func (s *Service) createStocktakeDrafts(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	orderNos := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		order, err := s.stocktake.Create(ctx, &stocktakedto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			Remark:      fmt.Sprintf("模拟盘点任务：第 %d 批", i+1),
		}, s.Username())
		if err != nil {
			return nil, err
		}
		orderNos = append(orderNos, order.OrderNo)
	}
	return &ScenarioResult{
		Name:        ScenarioStocktakeDrafts,
		Summary:     fmt.Sprintf("已创建 %d 张盘点草稿，请在盘点单页面录入实盘数量并审核", len(orderNos)),
		TargetPath:  "/stocktake/orders",
		TargetLabel: "前往盘点单处理",
		Steps: []ScenarioStep{
			{Title: "批量创建盘点单", Detail: fmt.Sprintf("共 %d 张，已生成库存快照", len(orderNos))},
			{Title: "下一步", Detail: "HR 可逐张进入详情录入实盘数量，再执行审核和库存调整"},
		},
	}, nil
}
