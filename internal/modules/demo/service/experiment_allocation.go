package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	invmodel "gowms/internal/modules/inventory/model"
	outbounddto "gowms/internal/modules/outbound/dto"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

type ConcurrentResult struct {
	Concurrency       int            `json:"concurrency"`
	QtyPerOrder       int            `json:"qty_per_order"`
	TotalDemand       int            `json:"total_demand"`
	StockBefore       int64          `json:"stock_before"`
	AvailableBefore   int64          `json:"available_before"`
	AllocatedBefore   int64          `json:"allocated_before"`
	CreatedOrders     int            `json:"created_orders"`
	SubmittedOrders   int            `json:"submitted_orders"`
	ApprovedOrders    int            `json:"approved_orders"`
	ApprovalFailed    int            `json:"approval_failed"`
	OtherFailed       int            `json:"other_failed"`
	AllocatedQuantity int64          `json:"allocated_quantity"`
	PickTaskCount     int            `json:"pick_task_count"`
	Success           int            `json:"success"`
	Failed            int            `json:"failed"`
	DurationMs        int64          `json:"duration_ms"`
	StockTotal        int64          `json:"stock_total"`
	AvailableTotal    int64          `json:"available_total"`
	AllocatedTotal    int64          `json:"allocated_total"`
	NegativeRows      int64          `json:"negative_rows"`
	InvariantOK       bool           `json:"invariant_ok"`
	InvariantMessage  string         `json:"invariant_message"`
	ValidationScope   string         `json:"validation_scope"`
	NotValidated      string         `json:"not_validated"`
	TaskStatsScope    string         `json:"task_stats_scope"`
	TestFocus         string         `json:"test_focus"`
	Summary           string         `json:"summary"`
	Steps             []ScenarioStep `json:"steps"`
}

type concurrentAttempt struct {
	OrderID      int64
	Created      bool
	Submitted    bool
	Approved     bool
	FailedPhase  string
	FailureCode  int
	FailureCause string
}

func (s *Service) RunConcurrentAllocation(ctx context.Context, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if concurrency <= 0 {
		concurrency = 20
	}
	if concurrency > 100 {
		concurrency = 100
	}
	if qtyPerOrder <= 0 {
		qtyPerOrder = 1
	}
	if qtyPerOrder > 10 {
		qtyPerOrder = 10
	}

	runCtx, finish, err := s.beginTenantRun(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer finish()
	ctx = runCtx

	refs, err := s.loadDemoBaseRefs(ctx)
	if err != nil {
		return nil, err
	}
	totalDemand := concurrency * qtyPerOrder
	statsBefore, err := s.concurrentInventoryStats(ctx, refs.SKU.ID)
	if err != nil {
		return nil, err
	}
	if statsBefore.AvailableTotal < int64(totalDemand) {
		return nil, errcode.New(errcode.DemoStockNotEnough.Code, fmt.Sprintf(
			"可用库存不足：当前可用 %d 件，本次并发出库需要 %d 件。这里的实验只验证库存充足条件下的一致性，请先补充库存或减少并发数量",
			statsBefore.AvailableTotal, totalDemand,
		))
	}

	start := time.Now()
	attempts := make(chan concurrentAttempt, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		index := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempts <- s.prepareConcurrentOutbound(ctx, refs, index, qtyPerOrder, "并发库存分配一致性实验", concurrentRunTag())
		}()
	}
	wg.Wait()
	close(attempts)

	attemptList := make([]concurrentAttempt, 0, concurrency)
	for attempt := range attempts {
		attemptList = append(attemptList, attempt)
	}
	summary := summarizeConcurrentAttempts(attemptList)
	createdOrders := summary.CreatedOrders
	submittedOrders := summary.SubmittedOrders
	approvedOrders := summary.ApprovedOrders
	approvalFailed := summary.ApprovalFailed
	otherFailed := summary.OtherFailed
	approvedOrderIDs := summary.ApprovedOrderIDs
	failReasons := summary.FailureReasons

	allocatedQuantity, pickTaskCount, err := s.concurrentAttemptEvidence(ctx, approvedOrderIDs)
	if err != nil {
		return nil, err
	}
	statsAfter, err := s.concurrentInventoryStats(ctx, refs.SKU.ID)
	if err != nil {
		return nil, err
	}
	invariantOK := concurrentInventoryInvariantOK(statsAfter)
	invariantMessage := "stock = available + allocated，且三个数量均非负"
	if !invariantOK {
		invariantMessage = fmt.Sprintf(
			"不变量异常：stock=%d available=%d allocated=%d，负数行=%d",
			statsAfter.StockTotal, statsAfter.AvailableTotal, statsAfter.AllocatedTotal, statsAfter.NegativeRows,
		)
	}

	validationScope := "库存充足条件下，多张出库单并发创建、提交和审核后，FIFO 分配结果与库存三数量仍保持一致。"
	notValidated := "不验证库存不足时的并发超卖防护，也不形成生产容量或高并发性能结论。"
	taskStatsScope := "PICK 任务和实际分配数量仅统计本次实验成功创建的出库单 ID。"
	testFocus := "并发创建/提交/审核、库存行锁、FIFO 分配、事务重试，以及库存充足场景下的一致性不变量"
	steps := []ScenarioStep{
		{Title: "实验前置", Detail: fmt.Sprintf("开始前 available=%d，总需求=%d，满足 available ≥ total demand", statsBefore.AvailableTotal, totalDemand), Status: ScenarioStepCompleted},
		{Title: "并发输入", Detail: fmt.Sprintf("订单数=%d，并发数=%d，每单需求=%d，总需求=%d", concurrency, concurrency, qtyPerOrder, totalDemand), Status: ScenarioStepCompleted},
		{Title: "单据阶段", Detail: fmt.Sprintf("创建成功=%d，提交成功=%d，审核成功=%d", createdOrders, submittedOrders, approvedOrders), Status: ScenarioStepCompleted},
		{Title: "审核结果", Detail: fmt.Sprintf("审核失败=%d，其他阶段失败=%d，实际分配=%d 件，PICK 任务=%d", approvalFailed, otherFailed, allocatedQuantity, pickTaskCount), Status: ScenarioStepCompleted},
		{Title: "库存不变量", Detail: fmt.Sprintf("结束 stock=%d available=%d allocated=%d；%s", statsAfter.StockTotal, statsAfter.AvailableTotal, statsAfter.AllocatedTotal, invariantMessage), Status: ScenarioStepCompleted},
		{Title: "验证边界", Detail: notValidated, Status: ScenarioStepCompleted},
	}
	if len(failReasons) > 0 {
		steps = append(steps, ScenarioStep{Title: "失败样例", Detail: failReasons[0], Status: ScenarioStepFailed, Error: failReasons[0]})
	}
	durationMs := time.Since(start).Milliseconds()
	resultSummary := fmt.Sprintf("并发库存分配一致性验证：审核成功 %d/%d，实际分配 %d 件，库存不变量%s",
		approvedOrders, concurrency, allocatedQuantity, map[bool]string{true: "成立", false: "异常"}[invariantOK])
	return &ConcurrentResult{
		Concurrency:       concurrency,
		QtyPerOrder:       qtyPerOrder,
		TotalDemand:       totalDemand,
		StockBefore:       statsBefore.StockTotal,
		AvailableBefore:   statsBefore.AvailableTotal,
		AllocatedBefore:   statsBefore.AllocatedTotal,
		CreatedOrders:     createdOrders,
		SubmittedOrders:   submittedOrders,
		ApprovedOrders:    approvedOrders,
		ApprovalFailed:    approvalFailed,
		OtherFailed:       otherFailed,
		AllocatedQuantity: allocatedQuantity,
		PickTaskCount:     pickTaskCount,
		Success:           approvedOrders,
		Failed:            concurrency - approvedOrders,
		DurationMs:        durationMs,
		StockTotal:        statsAfter.StockTotal,
		AvailableTotal:    statsAfter.AvailableTotal,
		AllocatedTotal:    statsAfter.AllocatedTotal,
		NegativeRows:      statsAfter.NegativeRows,
		InvariantOK:       invariantOK,
		InvariantMessage:  invariantMessage,
		ValidationScope:   validationScope,
		NotValidated:      notValidated,
		TaskStatsScope:    taskStatsScope,
		TestFocus:         testFocus,
		Summary:           resultSummary,
		Steps:             steps,
	}, nil
}

func (s *Service) prepareConcurrentOutbound(ctx context.Context, refs *demoRefs, index, qtyPerOrder int, remark, runTag string) concurrentAttempt {
	operator := s.Username(ctx)
	var attempt concurrentAttempt
	order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		BizOrderNo:  concurrentDemoBizOrderNo(runTag, index+1),
		Remark:      remark,
		Details:     []outbounddto.OrderDetailItem{{SKUID: refs.SKU.ID, ExpectedQty: qtyPerOrder}},
	}, operator)
	if err != nil {
		attempt.FailedPhase = "create"
		attempt.FailureCode = errcode.From(err).Code
		attempt.FailureCause = err.Error()
		return attempt
	}
	attempt.Created = true
	attempt.OrderID = order.ID

	if err := s.outbound.Submit(ctx, order.ID); err != nil {
		attempt.FailedPhase = "submit"
		attempt.FailureCode = errcode.From(err).Code
		attempt.FailureCause = err.Error()
		return attempt
	}
	attempt.Submitted = true

	if err := s.outbound.Approve(ctx, order.ID, operator); err != nil {
		attempt.FailedPhase = "approve"
		attempt.FailureCode = errcode.From(err).Code
		attempt.FailureCause = err.Error()
		return attempt
	}
	attempt.Approved = true
	return attempt
}

func (s *Service) concurrentAttemptEvidence(ctx context.Context, orderIDs []int64) (int64, int, error) {
	if len(orderIDs) == 0 {
		return 0, 0, nil
	}
	var allocatedQuantity int64
	if err := s.db.WithContext(ctx).Model(&outboundmodel.Allocation{}).
		Where("order_id IN ? AND status <> ?", orderIDs, outboundmodel.AllocCancelled).
		Select("COALESCE(SUM(allocated_qty), 0)").
		Scan(&allocatedQuantity).Error; err != nil {
		return 0, 0, err
	}
	var pickTaskCount int64
	if err := s.db.WithContext(ctx).Model(&taskmodel.Task{}).
		Where("order_id IN ? AND task_type = ? AND status <> ?", orderIDs, taskmodel.TaskPick, taskmodel.TaskCancelled).
		Count(&pickTaskCount).Error; err != nil {
		return 0, 0, err
	}
	return allocatedQuantity, int(pickTaskCount), nil
}

type concurrentAttemptSummary struct {
	CreatedOrders    int
	SubmittedOrders  int
	ApprovedOrders   int
	ApprovalFailed   int
	OtherFailed      int
	ApprovedOrderIDs []int64
	FailureReasons   []string
}

func summarizeConcurrentAttempts(attempts []concurrentAttempt) concurrentAttemptSummary {
	summary := concurrentAttemptSummary{
		ApprovedOrderIDs: make([]int64, 0, len(attempts)),
		FailureReasons:   make([]string, 0, 3),
	}
	for _, attempt := range attempts {
		if attempt.Created {
			summary.CreatedOrders++
		}
		if attempt.Submitted {
			summary.SubmittedOrders++
		}
		if attempt.Approved {
			summary.ApprovedOrders++
			summary.ApprovedOrderIDs = append(summary.ApprovedOrderIDs, attempt.OrderID)
			continue
		}
		if attempt.FailedPhase == "approve" {
			summary.ApprovalFailed++
		} else {
			summary.OtherFailed++
		}
		if attempt.FailureCause != "" && len(summary.FailureReasons) < 3 {
			summary.FailureReasons = append(summary.FailureReasons, attempt.FailureCause)
		}
	}
	return summary
}

type concurrentInventoryStats struct {
	StockTotal     int64
	AvailableTotal int64
	AllocatedTotal int64
	NegativeRows   int64
}

func concurrentInventoryInvariantOK(stats *concurrentInventoryStats) bool {
	if stats == nil {
		return false
	}
	return stats.NegativeRows == 0 && stats.StockTotal == stats.AvailableTotal+stats.AllocatedTotal
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

func concurrentRunTag() string {
	now := time.Now()
	return fmt.Sprintf("%s%06d", now.Format("20060102150405"), now.Nanosecond()/1000)
}

func concurrentDemoBizOrderNo(runTag string, sequence int) string {
	return fmt.Sprintf("CUST%s-%02d", runTag, sequence)
}
