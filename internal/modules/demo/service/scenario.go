package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gowms/internal/bootstrap"
	basicmodel "gowms/internal/modules/basic/model"
	inbounddto "gowms/internal/modules/inbound/dto"
	outbounddto "gowms/internal/modules/outbound/dto"
	stocktakedto "gowms/internal/modules/stocktake/dto"
	taskmodel "gowms/internal/modules/task/model"
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

// Run 先恢复默认演示数据，再执行指定场景。单实例内由演示会话锁保证只有
// 一个体验者能触发；runMu 进一步避免同一进程内的场景请求交叉执行。
func (s *Service) Run(ctx context.Context, sessionID, scenario string, options ...ScenarioOptions) (*ScenarioResult, error) {
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

func (s *Service) runInboundDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const qty = 10
	operator := s.Username(ctx)
	order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		Remark:      "一键演示：入库、收货、上架完整流程",
		Details: []inbounddto.OrderDetailItem{{
			SKUID: refs.SKU.ID, ExpectedQty: qty,
		}},
	}, operator)
	if err != nil {
		return nil, err
	}
	if err := s.inbound.Submit(ctx, order.ID); err != nil {
		return nil, err
	}
	if err := s.inbound.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	detail, err := s.inbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	if len(detail.Details) == 0 {
		return nil, errcode.DemoDataMissing
	}
	batchNo := demoBatchNo()
	if err := s.inbound.Receive(ctx, order.ID, detail.Details[0].ID, &inbounddto.ReceiveReq{
		DetailID: detail.Details[0].ID, Qty: qty, BatchNo: batchNo,
	}, operator); err != nil {
		return nil, err
	}

	detail, err = s.inbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	var putawayTaskID int64
	var putawayQty int
	for _, t := range detail.Tasks {
		if t.TaskType == taskmodel.TaskPutaway && t.Status != taskmodel.TaskCompleted && t.DoneQty < t.TargetQty {
			putawayTaskID = t.ID
			putawayQty = t.TargetQty - t.DoneQty
			break
		}
	}
	if putawayTaskID == 0 {
		return nil, errcode.DemoDataMissing
	}
	if err := s.inbound.Putaway(ctx, putawayTaskID, refs.Location.ID, putawayQty, operator); err != nil {
		return nil, err
	}

	return &ScenarioResult{
		Name:    ScenarioInbound,
		Summary: fmt.Sprintf("入库单 %s 已完成收货并上架 %d 件", order.OrderNo, qty),
		Steps: []ScenarioStep{
			{Title: "创建入库单", Detail: order.OrderNo},
			{Title: "提交并审核", Detail: "生成收货任务，状态机推进到 APPROVED"},
			{Title: "完成收货", Detail: fmt.Sprintf("货品 %s，批次 %s，数量 %d", refs.SKU.Code, batchNo, qty)},
			{Title: "完成上架", Detail: fmt.Sprintf("库位 %s，库存已增加", refs.Location.Code)},
		},
	}, nil
}

func (s *Service) runOutboundDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	const qty = 20
	operator := s.Username(ctx)
	order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		BizOrderNo:  demoBizOrderNo(1),
		Remark:      "一键演示：出库审核、FIFO 分配、拣货出库",
		Details: []outbounddto.OrderDetailItem{{
			SKUID: refs.SKU.ID, ExpectedQty: qty,
		}},
	}, operator)
	if err != nil {
		return nil, err
	}
	if err := s.outbound.Submit(ctx, order.ID); err != nil {
		return nil, err
	}
	if err := s.outbound.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	detail, err := s.outbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	var picked int
	for _, t := range detail.Tasks {
		if t.TaskType != taskmodel.TaskPick || t.Status == taskmodel.TaskCompleted {
			continue
		}
		pickQty := t.TargetQty - t.DoneQty
		if pickQty <= 0 {
			continue
		}
		if err := s.outbound.Pick(ctx, t.ID, pickQty, operator, nil); err != nil {
			return nil, err
		}
		picked += pickQty
	}
	if picked != qty {
		return nil, errcode.DemoDataMissing
	}

	return &ScenarioResult{
		Name:    ScenarioOutbound,
		Summary: fmt.Sprintf("出库单 %s 已完成 FIFO 分配并拣货 %d 件", order.OrderNo, qty),
		Steps: []ScenarioStep{
			{Title: "创建出库单", Detail: order.OrderNo},
			{Title: "提交并审核", Detail: "按入库时间 FIFO 分配库存并生成拣货任务"},
			{Title: "完成拣货", Detail: fmt.Sprintf("货品 %s，数量 %d，库存已扣减", refs.SKU.Code, qty)},
		},
	}, nil
}

func (s *Service) runStocktakeDemo(ctx context.Context, refs *demoRefs) (*ScenarioResult, error) {
	operator := s.Username(ctx)
	order, err := s.stocktake.Create(ctx, &stocktakedto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		Remark:      "一键演示：库存快照、录入实盘、审核调整",
	}, operator)
	if err != nil {
		return nil, err
	}
	detail, err := s.stocktake.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	if len(detail.Details) == 0 {
		return nil, errcode.DemoDataMissing
	}

	adjusted := false
	for i, item := range detail.Details {
		actual := item.BookQty
		if i == 0 && actual > 0 {
			actual--
			adjusted = true
		}
		if err := s.stocktake.RecordActual(ctx, order.ID, item.ID, actual); err != nil {
			return nil, err
		}
	}
	if !adjusted {
		return nil, errcode.DemoDataMissing
	}
	if err := s.stocktake.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	return &ScenarioResult{
		Name:    ScenarioStocktake,
		Summary: fmt.Sprintf("盘点单 %s 已完成实盘和库存调整", order.OrderNo),
		Steps: []ScenarioStep{
			{Title: "生成盘点快照", Detail: order.OrderNo},
			{Title: "录入实盘数量", Detail: fmt.Sprintf("共 %d 条库存明细", len(detail.Details))},
			{Title: "审核并调整", Detail: "差异数量已写入库存流水"},
		},
	}, nil
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
