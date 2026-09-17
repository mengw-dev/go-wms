package service

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	invmodel "gowms/internal/modules/inventory/model"
	outbounddto "gowms/internal/modules/outbound/dto"
)

// ConcurrentResult 一次受控并发业务演示的结果。
type ConcurrentResult struct {
	Concurrency    int            `json:"concurrency"`
	QtyPerOrder    int            `json:"qty_per_order"`
	TotalDemand    int            `json:"total_demand"`
	Success        int            `json:"success"`
	Failed         int            `json:"failed"`
	DurationMs     int64          `json:"duration_ms"`
	StockTotal     int64          `json:"stock_total"`
	AvailableTotal int64          `json:"available_total"`
	AllocatedTotal int64          `json:"allocated_total"`
	NegativeRows   int64          `json:"negative_rows"`
	TestFocus      string         `json:"test_focus"`
	Summary        string         `json:"summary"`
	Steps          []ScenarioStep `json:"steps"`
}

// RunConcurrent 恢复初始数据后并发执行多张出库单，演示库存锁、FIFO 和防超卖。
func (s *Service) RunConcurrent(ctx context.Context, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if concurrency <= 0 {
		concurrency = 20
	}
	if concurrency > 30 {
		concurrency = 30
	}
	if qtyPerOrder <= 0 {
		qtyPerOrder = 1
	}
	if qtyPerOrder > 10 {
		qtyPerOrder = 10
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()

	if err := s.resetLocked(ctx); err != nil {
		return nil, err
	}
	refs, err := s.loadDemoRefs(ctx)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	var success atomic.Int64
	var failed atomic.Int64
	errs := make(chan string, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		index := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.runConcurrentOutbound(ctx, refs, index, qtyPerOrder); err != nil {
				failed.Add(1)
				select {
				case errs <- err.Error():
				default:
				}
				return
			}
			success.Add(1)
		}()
	}
	wg.Wait()
	close(errs)

	stats, err := s.concurrentInventoryStats(ctx, refs.SKU.ID)
	if err != nil {
		return nil, err
	}
	failReasons := make([]string, 0, 3)
	for reason := range errs {
		if len(failReasons) >= 3 {
			break
		}
		failReasons = append(failReasons, reason)
	}
	totalDemand := concurrency * qtyPerOrder
	testFocus := "库存行锁、FIFO 分配、事务冲突重试、防超卖、防负库存"
	steps := []ScenarioStep{
		{Title: "并发模型", Detail: fmt.Sprintf("同时创建并处理 %d 张出库单，每张 %d 件，总需求 %d 件", concurrency, qtyPerOrder, totalDemand)},
		{Title: "测试重点", Detail: testFocus},
		{Title: "成功完成", Detail: fmt.Sprintf("%d 张出库单最终 SHIPPED", success.Load())},
		{Title: "业务拒绝", Detail: fmt.Sprintf("%d 张因库存锁/库存不足等业务规则被拒绝", failed.Load())},
		{Title: "最终库存", Detail: fmt.Sprintf("stock=%d available=%d allocated=%d 负数行=%d", stats.StockTotal, stats.AvailableTotal, stats.AllocatedTotal, stats.NegativeRows)},
	}
	if len(failReasons) > 0 {
		steps = append(steps, ScenarioStep{Title: "失败样例", Detail: failReasons[0]})
	}
	durationMs := time.Since(start).Milliseconds()
	summary := fmt.Sprintf("并发出库测试完成：%d 张 × %d 件，成功 %d，业务拒绝 %d，耗时 %dms", concurrency, qtyPerOrder, success.Load(), failed.Load(), durationMs)
	return &ConcurrentResult{
		Concurrency:    concurrency,
		QtyPerOrder:    qtyPerOrder,
		TotalDemand:    totalDemand,
		Success:        int(success.Load()),
		Failed:         int(failed.Load()),
		DurationMs:     durationMs,
		StockTotal:     stats.StockTotal,
		AvailableTotal: stats.AvailableTotal,
		AllocatedTotal: stats.AllocatedTotal,
		NegativeRows:   stats.NegativeRows,
		TestFocus:      testFocus,
		Summary:        summary,
		Steps:          steps,
	}, nil
}

func (s *Service) runConcurrentOutbound(ctx context.Context, refs *demoRefs, index, qtyPerOrder int) error {
	operator := s.Username()
	order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		BizOrderNo:  fmt.Sprintf("DEMO-CONCURRENT-%d-%d", time.Now().UnixNano(), index),
		Remark:      "并发业务演示：库存锁与防超卖",
		Details: []outbounddto.OrderDetailItem{{
			SKUID: refs.SKU.ID, ExpectedQty: qtyPerOrder,
		}},
	}, operator)
	if err != nil {
		return err
	}
	if err := s.outbound.Submit(ctx, order.ID); err != nil {
		return err
	}
	if err := s.outbound.Approve(ctx, order.ID, operator); err != nil {
		return err
	}
	detail, err := s.outbound.Get(ctx, order.ID)
	if err != nil {
		return err
	}
	for _, task := range detail.Tasks {
		qty := task.TargetQty - task.DoneQty
		if qty <= 0 {
			continue
		}
		if err := s.outbound.Pick(ctx, task.ID, qty, operator); err != nil {
			return err
		}
	}
	return nil
}

type concurrentInventoryStats struct {
	StockTotal     int64
	AvailableTotal int64
	AllocatedTotal int64
	NegativeRows   int64
}

func (s *Service) concurrentInventoryStats(ctx context.Context, skuID int64) (*concurrentInventoryStats, error) {
	var stats concurrentInventoryStats
	if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Where("sku_id = ?", skuID).
		Select("COALESCE(SUM(stock_quantity), 0) AS stock_total, COALESCE(SUM(available_quantity), 0) AS available_total, COALESCE(SUM(allocated_quantity), 0) AS allocated_total").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Where("sku_id = ? AND (stock_quantity < 0 OR available_quantity < 0 OR allocated_quantity < 0)", skuID).
		Count(&stats.NegativeRows).Error; err != nil {
		return nil, err
	}
	return &stats, nil
}
