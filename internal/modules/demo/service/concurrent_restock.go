package service

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	inbounddto "gowms/internal/modules/inbound/dto"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

func (s *Service) RestockDemo(ctx context.Context, sessionID string, qty int) (*ScenarioResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if qty <= 0 {
		qty = 500
	}
	if qty > 2000 {
		qty = 2000
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
	location, err := s.loadRestockLocation(ctx, refs.Warehouse.ID, refs.SKU.ID)
	if err != nil {
		return nil, err
	}

	operator := s.Username(ctx)
	order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		Remark:      fmt.Sprintf("一键补货入库：为并发演示补充 %d 件库存", qty),
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
	for _, task := range detail.Tasks {
		if task.TaskType == taskmodel.TaskPutaway && task.Status != taskmodel.TaskCompleted && task.DoneQty < task.TargetQty {
			putawayTaskID = task.ID
			putawayQty = task.TargetQty - task.DoneQty
			break
		}
	}
	if putawayTaskID == 0 {
		return nil, errcode.DemoDataMissing
	}
	if err := s.inbound.Putaway(ctx, putawayTaskID, location.ID, putawayQty, operator); err != nil {
		return nil, err
	}

	return &ScenarioResult{
		Name:        "restock",
		Summary:     fmt.Sprintf("一键补货完成：已通过完整入库流程补充 %d 件库存", qty),
		TargetPath:  "/inventory",
		TargetLabel: "查看库存",
		Steps: []ScenarioStep{
			{Title: "创建入库单", Detail: order.OrderNo},
			{Title: "提交并审核", Detail: "状态机推进到 APPROVED，生成收货任务"},
			{Title: "完成收货", Detail: fmt.Sprintf("货品 %s，批次 %s，数量 %d", refs.SKU.Code, batchNo, qty)},
			{Title: "完成上架", Detail: fmt.Sprintf("库位 %s，可用库存增加 %d 件", location.Code, qty)},
			{Title: "下一步", Detail: "点击“并发出库审核分配”，然后执行“PDA 并发拣货”"},
		},
	}, nil
}

func (s *Service) loadRestockLocation(ctx context.Context, warehouseID, skuID int64) (*basicmodel.Location, error) {
	var location basicmodel.Location
	query := s.db.WithContext(ctx).
		Table("wms_location AS l").
		Select("l.*").
		Joins("JOIN wms_inventory AS i ON i.location_id = l.id AND i.tenant_id = l.tenant_id AND i.deleted_at IS NULL").
		Where("l.deleted_at IS NULL AND l.warehouse_id = ? AND l.status <> ? AND i.sku_id = ?",
			warehouseID, basicmodel.LocationStatusDisabled, skuID)
	if tenantID, scoped := tenant.Scope(ctx); scoped {
		query = query.Where("l.tenant_id = ?", tenantID)
	}
	err := query.Order("i.stock_in_time ASC, l.code ASC").First(&location).Error
	if err == nil {
		return &location, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Where("warehouse_id = ? AND status <> ?", warehouseID, basicmodel.LocationStatusDisabled).
		Order("code ASC").
		First(&location).Error; err != nil {
		return nil, errcode.DemoDataMissing
	}
	return &location, nil
}
