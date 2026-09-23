package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	inventorymodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func TestScenarioRunRecordsCompletedSteps(t *testing.T) {
	run := newScenarioRun("test", "running",
		ScenarioStep{Title: "第一步", Detail: "创建业务对象"},
		ScenarioStep{Title: "第二步", Detail: "推进业务状态"},
	)

	if err := run.execute(0, "DRAFT → SUBMITTED", "service.Submit", func() (string, error) {
		return "ORDER-1", nil
	}); err != nil {
		t.Fatalf("execute first step: %v", err)
	}
	result := run.finish("执行完成")

	if result.Status != ScenarioStatusCompleted {
		t.Fatalf("result status = %q, want %q", result.Status, ScenarioStatusCompleted)
	}
	if result.Steps[0].Status != ScenarioStepCompleted {
		t.Fatalf("first step status = %q, want %q", result.Steps[0].Status, ScenarioStepCompleted)
	}
	if result.Steps[0].Object != "ORDER-1" {
		t.Fatalf("first step object = %q, want ORDER-1", result.Steps[0].Object)
	}
	if result.Steps[0].StatusChange != "DRAFT → SUBMITTED" {
		t.Fatalf("first step status change = %q", result.Steps[0].StatusChange)
	}
	if result.Steps[1].Status != ScenarioStepPending {
		t.Fatalf("second step status = %q, want %q", result.Steps[1].Status, ScenarioStepPending)
	}
}

func TestScenarioExecutionErrorKeepsCompletedAndFailedSteps(t *testing.T) {
	run := newScenarioRun("test", "running",
		ScenarioStep{Title: "创建订单", Detail: "创建订单"},
		ScenarioStep{Title: "提交订单", Detail: "提交订单"},
		ScenarioStep{Title: "审核订单", Detail: "审核订单"},
	)
	if err := run.execute(0, "DRAFT", "service.Create", func() (string, error) {
		return "ORDER-2", nil
	}); err != nil {
		t.Fatalf("execute completed step: %v", err)
	}

	err := run.execute(1, "DRAFT → SUBMITTED", "service.Submit", func() (string, error) {
		return "", errcode.OrderStatusWrong
	})
	if err == nil {
		t.Fatal("execute failed step returned nil error")
	}
	if !errors.Is(err, errcode.OrderStatusWrong) {
		t.Fatalf("error %v does not wrap business error", err)
	}

	var executionErr *ScenarioExecutionError
	if !errors.As(err, &executionErr) {
		t.Fatalf("error %T does not contain ScenarioExecutionError", err)
	}
	if executionErr.Result.Status != ScenarioStatusFailed {
		t.Fatalf("result status = %q, want %q", executionErr.Result.Status, ScenarioStatusFailed)
	}
	if executionErr.Result.Steps[0].Status != ScenarioStepCompleted {
		t.Fatalf("completed step status = %q", executionErr.Result.Steps[0].Status)
	}
	if executionErr.Result.Steps[1].Status != ScenarioStepFailed {
		t.Fatalf("failed step status = %q", executionErr.Result.Steps[1].Status)
	}
	if !strings.Contains(executionErr.Result.Steps[1].Error, errcode.OrderStatusWrong.Msg) {
		t.Fatalf("failed step error = %q, want %q", executionErr.Result.Steps[1].Error, errcode.OrderStatusWrong.Msg)
	}
	if executionErr.Result.Steps[2].Status != ScenarioStepPending {
		t.Fatalf("future step status = %q, want %q", executionErr.Result.Steps[2].Status, ScenarioStepPending)
	}
}

func TestInventoryStockChangeSummaryUsesThreeQuantityInvariant(t *testing.T) {
	trans := &inventorymodel.InventoryTrans{
		QuantityChange:  10,
		BeforeQuantity:  20,
		AfterQuantity:   30,
		AvailableBefore: 12,
		AvailableAfter:  20,
		TransType:       inventorymodel.TransReceive,
		TaskNo:          "TASK-1",
	}

	summary := inventoryStockChangeSummary(trans)
	want := "库存 +10 / 现存量 20 → 30 / 可用量 12 → 20 / 已分配 8 → 10"
	if summary != want {
		t.Fatalf("summary = %q, want %q", summary, want)
	}

	facts := stockChangeFacts(trans)
	if len(facts) != 6 {
		t.Fatalf("facts count = %d, want 6", len(facts))
	}
	if facts[0].Label != "库存变化" || facts[0].Value != "+10" {
		t.Fatalf("stock change fact = %#v", facts[0])
	}
	if facts[5].Label != "流水任务" || facts[5].Value != "TASK-1" {
		t.Fatalf("task fact = %#v", facts[5])
	}
}

func TestOutboundFIFOFactsUseRealAllocationsInStockInOrder(t *testing.T) {
	early := &inventorymodel.Inventory{
		BatchNo:      "B-EARLY",
		AvailableQty: 0,
		StockInTime:  time.Date(2026, 9, 1, 9, 0, 0, 0, time.Local),
	}
	early.ID = 101
	late := &inventorymodel.Inventory{
		BatchNo:      "B-LATE",
		AvailableQty: 50,
		StockInTime:  time.Date(2026, 9, 5, 9, 0, 0, 0, time.Local),
	}
	late.ID = 102

	lateAllocation := &outboundmodel.Allocation{
		InventoryID:  late.ID,
		LocationCode: "A01-01-01",
		BatchNo:      "B-LATE",
		AllocatedQty: 20,
	}
	lateAllocation.ID = 2
	earlyAllocation := &outboundmodel.Allocation{
		InventoryID:  early.ID,
		LocationCode: "A01-01-01",
		BatchNo:      "B-EARLY",
		AllocatedQty: 30,
	}
	earlyAllocation.ID = 1

	inventories := map[int64]*inventorymodel.Inventory{early.ID: early, late.ID: late}
	facts := outboundFIFOFacts([]*outboundmodel.Allocation{lateAllocation, earlyAllocation}, inventories, 50)

	if len(facts) != 3 {
		t.Fatalf("facts count = %d, want 3", len(facts))
	}
	if facts[0].Label != "分配汇总" || facts[0].Value != "需求 50 / 已分配 50 / 2 个批次" {
		t.Fatalf("summary fact = %#v", facts[0])
	}
	if !strings.Contains(facts[1].Value, "B-EARLY") || !strings.Contains(facts[1].Value, "本次分配 30") {
		t.Fatalf("first FIFO fact = %q, want early batch", facts[1].Value)
	}
	if !strings.Contains(facts[2].Value, "B-LATE") || !strings.Contains(facts[2].Value, "本次分配 20") {
		t.Fatalf("second FIFO fact = %q, want late batch", facts[2].Value)
	}
}

func TestOutboundPickFactsUseCompletedRealTasks(t *testing.T) {
	task := &taskmodel.Task{
		TaskNo:       "PICK-001",
		TaskType:     taskmodel.TaskPick,
		Status:       taskmodel.TaskCompleted,
		LocationCode: "A01-01-01",
		BatchNo:      "B-EARLY",
		TargetQty:    30,
		DoneQty:      30,
	}

	facts := outboundPickTaskFacts([]*taskmodel.Task{task})
	if len(facts) != 2 {
		t.Fatalf("facts count = %d, want 2", len(facts))
	}
	if facts[0].Label != "PICK 任务数量" || facts[0].Value != "1 个" {
		t.Fatalf("task count fact = %#v", facts[0])
	}
	if !strings.Contains(facts[1].Value, "PICK-001") || !strings.Contains(facts[1].Value, "完成 30/30") {
		t.Fatalf("task fact = %q", facts[1].Value)
	}
}

func TestSummarizeOutboundTransUsesAllocationAndShipValues(t *testing.T) {
	trans := []*inventorymodel.InventoryTrans{
		{
			TransType:       inventorymodel.TransAllocate,
			BeforeQuantity:  80,
			AfterQuantity:   80,
			AvailableBefore: 80,
			AvailableAfter:  30,
		},
		{
			TransType:       inventorymodel.TransShip,
			QuantityChange:  -50,
			BeforeQuantity:  80,
			AfterQuantity:   30,
			AvailableBefore: 30,
			AvailableAfter:  30,
		},
	}

	summary := summarizeOutboundTrans(trans)
	if summary.AllocateCount != 1 || summary.ShipCount != 1 {
		t.Fatalf("trans counts = %d/%d, want 1/1", summary.AllocateCount, summary.ShipCount)
	}
	if summary.StockChange != -50 || summary.StockBefore != 80 || summary.StockAfter != 30 {
		t.Fatalf("stock summary = %#v", summary)
	}
	if summary.AvailableBefore != 80 || summary.AvailableAfter != 30 {
		t.Fatalf("available summary = %#v", summary)
	}
	if summary.AllocatedBefore != 0 || summary.AllocatedAfter != 0 {
		t.Fatalf("allocated summary = %#v", summary)
	}
}

func TestMergeScenarioResultsKeepsInboundEvidence(t *testing.T) {
	implementation := &ScenarioImplementation{
		Orchestration: "scenario_inbound.go",
		BusinessFiles: []string{"receiving.go"},
		CallChain:     []string{"Demo Orchestrator", "Inbound Service"},
	}
	inbound := &ScenarioResult{
		Name:           ScenarioInbound,
		Status:         ScenarioStatusCompleted,
		EvidenceTitle:  "本次入库产生",
		Evidence:       []ScenarioEvidence{{Label: "库存流水", Value: "+10"}},
		Links:          []ScenarioLink{{Label: "查看库存流水", Path: "/inventory"}},
		Implementation: implementation,
	}

	merged := mergeScenarioResults(inbound, &ScenarioResult{Name: ScenarioOutbound, Status: ScenarioStatusCompleted})

	if merged.EvidenceTitle != "本次入库产生" || len(merged.Evidence) != 1 {
		t.Fatalf("merged evidence = %#v", merged.Evidence)
	}
	if len(merged.Links) != 1 || merged.Implementation != implementation {
		t.Fatalf("merged links/implementation = %#v / %#v", merged.Links, merged.Implementation)
	}
}

func TestMergeScenarioResultsUsesGenericTitleForMultipleEvidenceSets(t *testing.T) {
	merged := mergeScenarioResults(
		&ScenarioResult{
			Name:          ScenarioInbound,
			EvidenceTitle: "本次入库产生",
			Evidence:      []ScenarioEvidence{{Label: "入库单", Value: "RK-1"}},
		},
		&ScenarioResult{
			Name:          ScenarioOutbound,
			EvidenceTitle: "本次出库产生",
			Evidence:      []ScenarioEvidence{{Label: "出库单", Value: "CK-1"}},
		},
	)

	if merged.EvidenceTitle != "本次完整业务闭环产生" {
		t.Fatalf("evidence title = %q", merged.EvidenceTitle)
	}
	if len(merged.Evidence) != 2 {
		t.Fatalf("evidence count = %d, want 2", len(merged.Evidence))
	}
}
