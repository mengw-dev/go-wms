package service

import (
	"errors"
	"strings"
	"testing"

	inventorymodel "gowms/internal/modules/inventory/model"
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
