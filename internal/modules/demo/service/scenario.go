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

// ScenarioStep 演示中的一个可展示步骤。
type ScenarioStep struct {
	Title  string `json:"title"`
	Detail string `json:"detail"`
}

// ScenarioResult 一次演示场景的执行结果。
type ScenarioResult struct {
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	TargetPath  string         `json:"target_path,omitempty"`
	TargetLabel string         `json:"target_label,omitempty"`
	Steps       []ScenarioStep `json:"steps"`
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
			return nil, err
		}
		outbound, err := s.runOutboundDemo(ctx, refs)
		if err != nil {
			return nil, err
		}
		stocktake, err := s.runStocktakeDemo(ctx, refs)
		if err != nil {
			return nil, err
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
		Steps:   make([]ScenarioStep, 0),
	}
	for _, result := range results {
		if result == nil {
			continue
		}
		merged.Steps = append(merged.Steps, result.Steps...)
	}
	return merged
}
