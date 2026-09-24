package service

import (
	"context"
	"fmt"
	"sync"

	"gowms/internal/pkg/errcode"
)

// ConcurrentShortageResult 是供给不足并发验证的稳定响应。
// 该实验只说明本次真实调用中成功分配量是否受初始可用库存限制，不构成容量或生产级性能结论。
type ConcurrentShortageResult struct {
	Concurrency          int            `json:"concurrency"`
	QtyPerOrder          int            `json:"qty_per_order"`
	TotalDemand          int            `json:"total_demand"`
	StockBefore          int64          `json:"stock_before"`
	AvailableBefore      int64          `json:"available_before"`
	AllocatedBefore      int64          `json:"allocated_before"`
	Success              int            `json:"success"`
	InsufficientRejected int            `json:"insufficient_rejected"`
	OtherFailed          int            `json:"other_failed"`
	AllocatedQuantity    int64          `json:"allocated_quantity"`
	RemainingAvailable   int64          `json:"remaining_available"`
	StockTotal           int64          `json:"stock_total"`
	AvailableTotal       int64          `json:"available_total"`
	AllocatedTotal       int64          `json:"allocated_total"`
	NegativeRows         int64          `json:"negative_rows"`
	LimitRespected       bool           `json:"limit_respected"`
	InvariantOK          bool           `json:"invariant_ok"`
	InvariantMessage     string         `json:"invariant_message"`
	ValidationScope      string         `json:"validation_scope"`
	NotValidated         string         `json:"not_validated"`
	Summary              string         `json:"summary"`
	Steps                []ScenarioStep `json:"steps"`
}

type concurrentShortageSummary struct {
	Success              int
	InsufficientRejected int
	OtherFailed          int
}

func summarizeShortageAttempts(attempts []concurrentAttempt) concurrentShortageSummary {
	var summary concurrentShortageSummary
	for _, attempt := range attempts {
		if attempt.Approved {
			summary.Success++
			continue
		}
		if attempt.FailedPhase == "approve" && attempt.FailureCode == errcode.AvailableNotEnough.Code {
			summary.InsufficientRejected++
			continue
		}
		summary.OtherFailed++
	}
	return summary
}

// RunConcurrentShortageValidation 使用真实 Outbound Service 并发创建、提交和审核出库单，
// 并刻意让总需求大于初始可用库存。没有直接修改库存表，也没有绕过业务事务。
func (s *Service) RunConcurrentShortageValidation(ctx context.Context, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentShortageResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if concurrency <= 1 {
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

	s.runMu.Lock()
	defer s.runMu.Unlock()
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
	if statsBefore.AvailableTotal <= 0 {
		return nil, errcode.New(errcode.DemoStockNotEnough.Code,
			"当前可用库存为 0，无法构造供给不足并发验证，请先通过真实入库流程补充库存")
	}
	if int64(totalDemand) <= statsBefore.AvailableTotal {
		return nil, errcode.New(errcode.ParamError.Code, fmt.Sprintf(
			"总需求 %d 件未超过当前可用库存 %d 件，请增加并发订单数或单笔需求后再执行",
			totalDemand, statsBefore.AvailableTotal,
		))
	}

	attempts := make(chan concurrentAttempt, concurrency)
	runTag := concurrentRunTag()
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		index := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempts <- s.prepareConcurrentOutbound(ctx, refs, index, qtyPerOrder, "供给不足并发库存验证", runTag)
		}()
	}
	wg.Wait()
	close(attempts)

	attemptList := make([]concurrentAttempt, 0, concurrency)
	for attempt := range attempts {
		attemptList = append(attemptList, attempt)
	}
	attemptSummary := summarizeConcurrentAttempts(attemptList)
	shortageSummary := summarizeShortageAttempts(attemptList)
	allocatedQuantity, _, err := s.concurrentAttemptEvidence(ctx, attemptSummary.ApprovedOrderIDs)
	if err != nil {
		return nil, err
	}
	statsAfter, err := s.concurrentInventoryStats(ctx, refs.SKU.ID)
	if err != nil {
		return nil, err
	}

	limitRespected := allocatedQuantity <= statsBefore.AvailableTotal
	invariantOK := limitRespected && concurrentInventoryInvariantOK(statsAfter)
	invariantMessage := fmt.Sprintf(
		"成功分配 %d ≤ 初始可用 %d；stock = available + allocated，且三个数量均非负",
		allocatedQuantity, statsBefore.AvailableTotal,
	)
	if !invariantOK {
		invariantMessage = fmt.Sprintf(
			"校验失败：成功分配=%d，初始可用=%d，结束 stock=%d available=%d allocated=%d，负数行=%d",
			allocatedQuantity, statsBefore.AvailableTotal,
			statsAfter.StockTotal, statsAfter.AvailableTotal, statsAfter.AllocatedTotal, statsAfter.NegativeRows,
		)
	}

	validationScope := "总需求大于初始可用库存时，并发执行真实出库创建、提交和审核，核对成功分配量、库存不足拒绝与最终三数量。"
	notValidated := "该实验只反映本次受限并发输入下的真实结果，不用于宣称“绝对不会超卖”或证明生产环境容量。"
	result := &ConcurrentShortageResult{
		Concurrency:          concurrency,
		QtyPerOrder:          qtyPerOrder,
		TotalDemand:          totalDemand,
		StockBefore:          statsBefore.StockTotal,
		AvailableBefore:      statsBefore.AvailableTotal,
		AllocatedBefore:      statsBefore.AllocatedTotal,
		Success:              attemptSummary.ApprovedOrders,
		InsufficientRejected: shortageSummary.InsufficientRejected,
		OtherFailed:          shortageSummary.OtherFailed,
		AllocatedQuantity:    allocatedQuantity,
		RemainingAvailable:   statsAfter.AvailableTotal,
		StockTotal:           statsAfter.StockTotal,
		AvailableTotal:       statsAfter.AvailableTotal,
		AllocatedTotal:       statsAfter.AllocatedTotal,
		NegativeRows:         statsAfter.NegativeRows,
		LimitRespected:       limitRespected,
		InvariantOK:          invariantOK,
		InvariantMessage:     invariantMessage,
		ValidationScope:      validationScope,
		NotValidated:         notValidated,
	}
	result.Summary = fmt.Sprintf(
		"供给不足并发验证：总需求 %d，成功 %d，库存不足拒绝 %d，其他失败 %d，成功分配 %d，剩余可用 %d",
		totalDemand, result.Success, result.InsufficientRejected, result.OtherFailed, result.AllocatedQuantity, result.RemainingAvailable,
	)
	result.Steps = []ScenarioStep{
		{Title: "实验前置", Detail: fmt.Sprintf("初始 available=%d，总需求=%d，满足 total demand > available", statsBefore.AvailableTotal, totalDemand), Status: ScenarioStepCompleted},
		{Title: "真实调用", Detail: fmt.Sprintf("并发创建并按真实业务审核 %d 张出库单，每张需求 %d 件", concurrency, qtyPerOrder), Status: ScenarioStepCompleted},
		{Title: "业务结果", Detail: fmt.Sprintf("成功 %d，库存不足拒绝 %d，其他失败 %d", result.Success, result.InsufficientRejected, result.OtherFailed), Status: ScenarioStepCompleted},
		{Title: "库存结果", Detail: fmt.Sprintf("成功分配=%d，结束 available=%d，allocated=%d，负数行=%d", result.AllocatedQuantity, result.AvailableTotal, result.AllocatedTotal, result.NegativeRows), Status: ScenarioStepCompleted},
		{Title: "校验结论", Detail: invariantMessage, Status: ScenarioStepCompleted},
		{Title: "验证边界", Detail: notValidated, Status: ScenarioStepCompleted},
	}
	return result, nil
}
