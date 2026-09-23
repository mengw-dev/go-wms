package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gowms/internal/bootstrap"
	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

// 演示场景标识。
const (
	ScenarioInbound         = "inbound"
	ScenarioOutbound        = "outbound"
	ScenarioStocktake       = "stocktake"
	ScenarioFull            = "full"
	ScenarioInboundDrafts   = "inbound_drafts"
	ScenarioOutboundDrafts  = "outbound_drafts"
	ScenarioStocktakeDrafts = "stocktake_drafts"
)

// 演示执行结果的步骤与整体状态。步骤由后端按真实业务调用顺序记录，
// 前端只回放这些结果，不根据本地计时器推断执行进度。
const (
	ScenarioStatusCompleted = "completed"
	ScenarioStatusFailed    = "failed"

	ScenarioStepPending   = "pending"
	ScenarioStepCompleted = "completed"
	ScenarioStepFailed    = "failed"
)

// ScenarioStep 演示中的一个可展示步骤。
type ScenarioStep struct {
	Title        string `json:"title"`
	Detail       string `json:"detail"`
	Status       string `json:"status"`
	Object       string `json:"object,omitempty"`
	DurationMs   int64  `json:"duration_ms,omitempty"`
	StatusChange string `json:"status_change,omitempty"`
	Technical    string `json:"technical,omitempty"`
	Error        string `json:"error,omitempty"`
}

// ScenarioResult 一次演示场景的执行结果。
type ScenarioResult struct {
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	Status      string         `json:"status"`
	TargetPath  string         `json:"target_path,omitempty"`
	TargetLabel string         `json:"target_label,omitempty"`
	Steps       []ScenarioStep `json:"steps"`
}

// ScenarioExecutionError 表示真实业务步骤已经产生可展示结果但执行失败。
// Handler 会保留原业务错误码，同时把已执行步骤和失败步骤返回给前端。
type ScenarioExecutionError struct {
	Result *ScenarioResult
	Err    error
}

func (e *ScenarioExecutionError) Error() string {
	if e == nil || e.Err == nil {
		return "scenario execution failed"
	}
	return e.Err.Error()
}

func (e *ScenarioExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// scenarioRun 累积一次场景的真实步骤状态。每个步骤在调用前预置为 pending，
// execute 成功后记录对象、耗时和完成状态；失败时保留已完成步骤并标记失败原因。
type scenarioRun struct {
	result *ScenarioResult
}

func newScenarioRun(name, summary string, steps ...ScenarioStep) *scenarioRun {
	for i := range steps {
		if steps[i].Status == "" {
			steps[i].Status = ScenarioStepPending
		}
	}
	return &scenarioRun{result: &ScenarioResult{
		Name:    name,
		Summary: summary,
		Status:  ScenarioStatusCompleted,
		Steps:   steps,
	}}
}

func (r *scenarioRun) execute(index int, statusChange, technical string, fn func() (string, error)) error {
	if index < 0 || index >= len(r.result.Steps) {
		return errcode.Internal
	}
	step := &r.result.Steps[index]
	step.StatusChange = statusChange
	step.Technical = technical
	startedAt := time.Now()
	object, err := fn()
	step.DurationMs = time.Since(startedAt).Milliseconds()
	if err != nil {
		step.Status = ScenarioStepFailed
		step.Error = errcode.From(err).Msg
		r.result.Status = ScenarioStatusFailed
		return &ScenarioExecutionError{Result: r.result, Err: err}
	}
	if object != "" {
		step.Object = object
	}
	step.Status = ScenarioStepCompleted
	return nil
}

func (r *scenarioRun) finish(summary string) *ScenarioResult {
	if strings.TrimSpace(summary) != "" {
		r.result.Summary = summary
	}
	r.result.Status = ScenarioStatusCompleted
	return r.result
}

// ScenarioOptions 批量草稿类场景的可选参数。
type ScenarioOptions struct {
	Count int `json:"count"`
	Qty   int `json:"qty"`
}

// RunScenario 先恢复默认演示数据，再执行指定场景。单实例内由演示会话锁保证只有
// 一个体验者能触发；runMu 进一步避免同一进程内的场景请求交叉执行。
func (s *Service) RunScenario(ctx context.Context, sessionID, scenario string, options ...ScenarioOptions) (*ScenarioResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	scenario = strings.ToLower(strings.TrimSpace(scenario))
	switch scenario {
	case ScenarioInbound, ScenarioOutbound, ScenarioStocktake, ScenarioFull,
		ScenarioInboundDrafts, ScenarioOutboundDrafts, ScenarioStocktakeDrafts:
	default:
		return nil, errcode.ParamError
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()
	runCtx, finish, err := s.beginTenantRun(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer finish()
	ctx = runCtx

	if scenario == ScenarioInboundDrafts || scenario == ScenarioOutboundDrafts || scenario == ScenarioStocktakeDrafts {
		refs, err := s.loadDemoBaseRefs(ctx)
		if err != nil {
			return nil, err
		}
		var opt ScenarioOptions
		if len(options) > 0 {
			opt = options[0]
		}
		switch scenario {
		case ScenarioInboundDrafts:
			return s.createInboundDrafts(ctx, refs, opt.Count, opt.Qty)
		case ScenarioOutboundDrafts:
			return s.createOutboundDrafts(ctx, refs, opt.Count, opt.Qty)
		default:
			return s.createStocktakeDrafts(ctx, refs, opt.Count)
		}
	}

	if err := bootstrap.ResetDemoData(ctx, s.db, tenant.FromContext(ctx)); err != nil {
		return nil, err
	}
	refs, err := s.loadDemoRefs(ctx)
	if err != nil {
		return nil, err
	}

	switch scenario {
	case ScenarioInbound:
		return s.runInboundDemo(ctx, refs)
	case ScenarioOutbound:
		return s.runOutboundDemo(ctx, refs)
	case ScenarioStocktake:
		return s.runStocktakeDemo(ctx, refs)
	case ScenarioFull:
		inbound, err := s.runInboundDemo(ctx, refs)
		if err != nil {
			return inbound, err
		}
		outbound, err := s.runOutboundDemo(ctx, refs)
		if err != nil {
			return mergeScenarioResults(inbound, outbound), err
		}
		stocktake, err := s.runStocktakeDemo(ctx, refs)
		if err != nil {
			return mergeScenarioResults(inbound, outbound, stocktake), err
		}
		return mergeScenarioResults(inbound, outbound, stocktake), nil
	default:
		return nil, errcode.ParamError
	}
}

type demoRefs struct {
	Warehouse basicmodel.Warehouse
	SKU       basicmodel.SKU
	Location  basicmodel.Location
}

func (s *Service) loadDemoBaseRefs(ctx context.Context) (*demoRefs, error) {
	var refs demoRefs
	if err := s.db.WithContext(ctx).Where("code = ? AND status = ?", "WH01", 1).
		First(&refs.Warehouse).Error; err != nil {
		return nil, errcode.DemoDataMissing
	}
	if err := s.db.WithContext(ctx).Where("code = ? AND status = ?", "SKU000001", 1).
		First(&refs.SKU).Error; err != nil {
		return nil, errcode.DemoDataMissing
	}
	return &refs, nil
}

func (s *Service) loadDemoRefs(ctx context.Context) (*demoRefs, error) {
	refs, err := s.loadDemoBaseRefs(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("warehouse_id = ? AND status = ?", refs.Warehouse.ID, basicmodel.LocationStatusIdle).
		Order("code ASC").First(&refs.Location).Error; err != nil {
		return nil, errcode.DemoDataMissing
	}
	return refs, nil
}

// demoBatchNo 生成正常格式的批次号，例如 B20260918150405。
// 演示数据需要与真实业务数据外观一致，不带任何演示标记。
func demoBatchNo() string {
	return "B" + time.Now().Format("20060102150405")
}

// demoBizOrderNo 生成正常格式的上游业务单号，例如 CUST20260918150405-01。
// seq 用于同一次批量操作内区分多张单据。
func demoBizOrderNo(seq int) string {
	return fmt.Sprintf("CUST%s-%02d", time.Now().Format("20060102150405"), seq)
}

func mergeScenarioResults(results ...*ScenarioResult) *ScenarioResult {
	merged := &ScenarioResult{
		Name:    ScenarioFull,
		Summary: "入库、出库、盘点三个核心流程已全部完成",
		Status:  ScenarioStatusCompleted,
		Steps:   make([]ScenarioStep, 0),
	}
	for _, result := range results {
		if result == nil {
			continue
		}
		merged.Steps = append(merged.Steps, result.Steps...)
		if result.Status == ScenarioStatusFailed {
			merged.Status = ScenarioStatusFailed
		}
	}
	if merged.Status == ScenarioStatusFailed {
		merged.Summary = "完整业务闭环未全部完成，已执行的步骤保留在下方"
	}
	return merged
}
