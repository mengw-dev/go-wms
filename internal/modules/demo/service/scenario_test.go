package service

import (
	"errors"
	"strings"
	"testing"
	"time"

	inventorymodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
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

func TestMergeScenarioResultsCombinesTechnicalImplementation(t *testing.T) {
	inbound := &ScenarioImplementation{
		Orchestration: "scenario_inbound.go",
		BusinessFiles: []string{"receiving.go", "stock.go"},
		CallChain:     []string{"Demo Orchestrator", "Inbound Service", "Inventory Service", "MySQL"},
	}
	outbound := &ScenarioImplementation{
		Orchestration: "scenario_outbound.go",
		BusinessFiles: []string{"pick.go", "stock.go"},
		CallChain:     []string{"Demo Orchestrator", "Outbound Service", "Inventory / Task", "MySQL"},
	}
	stocktake := &ScenarioImplementation{
		Orchestration: "scenario_stocktake.go",
		BusinessFiles: []string{"approve.go"},
		CallChain:     []string{"Demo Orchestrator", "Stocktake Service", "Inventory Service", "MySQL"},
	}

	merged := mergeScenarioResults(
		&ScenarioResult{Name: ScenarioInbound, Implementation: inbound},
		&ScenarioResult{Name: ScenarioOutbound, Implementation: outbound},
		&ScenarioResult{Name: ScenarioStocktake, Implementation: stocktake},
	)

	if merged.Implementation == nil {
		t.Fatal("merged implementation is nil")
	}
	if merged.Implementation.Orchestration != "internal/modules/demo/service/scenario.go" {
		t.Fatalf("merged orchestration = %q", merged.Implementation.Orchestration)
	}
	if got, want := strings.Join(merged.Implementation.BusinessFiles, ","), "receiving.go,stock.go,pick.go,approve.go"; got != want {
		t.Fatalf("merged business files = %q, want %q", got, want)
	}
	if got, want := strings.Join(merged.Implementation.CallChain, " > "), "Demo Orchestrator > Inbound / Outbound / Stocktake Service > Inventory / Task > Transaction / MySQL"; got != want {
		t.Fatalf("merged call chain = %q, want %q", got, want)
	}
	if len(inbound.BusinessFiles) != 2 || len(outbound.BusinessFiles) != 2 || len(stocktake.BusinessFiles) != 1 {
		t.Fatal("mergeScenarioResults mutated source implementation files")
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

func TestStocktakePresentationUsesRecordedBookAndActualQuantities(t *testing.T) {
	actual27 := 27
	actual70 := 70
	details := []*stocktakemodel.StocktakeDetail{
		{
			SKUID: 7, SKUCode: "SKU-7", BatchNo: "B-1", LocationCode: "L-1",
			BookQty: 30, ActualQty: &actual27,
		},
		{
			SKUID: 7, SKUCode: "SKU-7", BatchNo: "B-2", LocationCode: "L-2",
			BookQty: 70, ActualQty: &actual70,
		},
		{
			SKUID: 8, SKUCode: "SKU-8", BatchNo: "B-3", LocationCode: "L-3",
			BookQty: 50,
		},
	}

	summary := summarizeStocktakeDetails(details, 7)
	if summary.BookQty != 100 || summary.ActualQty != 97 || summary.DiffQty != -3 {
		t.Fatalf("stocktake summary = %#v", summary)
	}
	if summary.Lines != 2 || summary.Counted != 2 {
		t.Fatalf("stocktake detail counts = %#v", summary)
	}

	snapshotFacts := stocktakeSnapshotFacts(details, 7)
	if len(snapshotFacts) != 3 {
		t.Fatalf("snapshot facts count = %d, want 3", len(snapshotFacts))
	}
	if snapshotFacts[0].Value != "账面合计 100 件 / 2 条批次" {
		t.Fatalf("snapshot summary fact = %#v", snapshotFacts[0])
	}
	if !strings.Contains(snapshotFacts[1].Value, "批次 B-1") ||
		!strings.Contains(snapshotFacts[1].Value, "账面 30") {
		t.Fatalf("first snapshot fact = %q", snapshotFacts[1].Value)
	}

	actualFacts := stocktakeActualFacts(details, 7)
	if len(actualFacts) != 3 {
		t.Fatalf("actual facts count = %d, want 3", len(actualFacts))
	}
	if actualFacts[0].Label != "实盘明细" || actualFacts[0].Value != "2 条" {
		t.Fatalf("actual summary fact = %#v", actualFacts[0])
	}
	if !strings.Contains(actualFacts[1].Value, "实盘 27") ||
		!strings.Contains(actualFacts[1].Value, "差异 -3") {
		t.Fatalf("first actual line fact = %q", actualFacts[1].Value)
	}

	differenceFacts := stocktakeDifferenceFacts(summary)
	if len(differenceFacts) != 3 {
		t.Fatalf("difference facts count = %d, want 3", len(differenceFacts))
	}
	if differenceFacts[0].Label != "账面数量" || differenceFacts[0].Value != "100 件" {
		t.Fatalf("book fact = %#v", differenceFacts[0])
	}
	if differenceFacts[1].Label != "实盘数量" || differenceFacts[1].Value != "97 件" {
		t.Fatalf("actual fact = %#v", differenceFacts[1])
	}
	if differenceFacts[2].Label != "盘点差异" || differenceFacts[2].Value != "-3" {
		t.Fatalf("difference fact = %#v", differenceFacts[2])
	}
}

func TestStocktakeAdjustmentFactsUsePersistedInventoryTrans(t *testing.T) {
	trans := &inventorymodel.InventoryTrans{
		TransType:       inventorymodel.TransAdjust,
		QuantityChange:  -3,
		BeforeQuantity:  30,
		AfterQuantity:   27,
		AvailableBefore: 30,
		AvailableAfter:  27,
		OrderNo:         "PD-1",
	}

	facts := stocktakeAdjustmentFacts(trans)
	if len(facts) != 5 {
		t.Fatalf("adjustment facts count = %d, want 5", len(facts))
	}
	if facts[0].Label != "库存流水" || facts[0].Value != "ADJUST -3" {
		t.Fatalf("inventory trans fact = %#v", facts[0])
	}
	if facts[1].Value != "30 → 27" || facts[2].Value != "30 → 27" {
		t.Fatalf("stock/available facts = %#v / %#v", facts[1], facts[2])
	}
	if facts[4].Label != "流水单号" || facts[4].Value != "PD-1" {
		t.Fatalf("order fact = %#v", facts[4])
	}
}
