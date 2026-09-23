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
	run := newScenarioRun(
		"restock",
		fmt.Sprintf("正在通过真实入库 Service 补充 %d 件库存", qty),
		ScenarioStep{Title: "创建补货入库单", Detail: fmt.Sprintf("创建 %d 件补货草稿", qty)},
		ScenarioStep{Title: "提交补货入库单", Detail: "将补货单从草稿推进到已提交状态"},
		ScenarioStep{Title: "审核补货入库单", Detail: "审核通过并生成收货任务"},
		ScenarioStep{Title: "完成补货收货", Detail: "登记真实批次和收货数量"},
		ScenarioStep{Title: "完成补货上架", Detail: "上架到目标库位并增加可用库存"},
	)

	var orderID int64
	var orderNo string
	if err := run.execute(0, "DRAFT", "inbound.Service.Create", func() (string, error) {
		order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
			WarehouseID: refs.Warehouse.ID,
			Remark:      fmt.Sprintf("一键补货入库：为并发演示补充 %d 件库存", qty),
			Details: []inbounddto.OrderDetailItem{{
				SKUID: refs.SKU.ID, ExpectedQty: qty,
			}},
		}, operator)
		if err != nil {
			return "", err
		}
		orderID = order.ID
		orderNo = order.OrderNo
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}
	if err := run.execute(1, "DRAFT → SUBMITTED", "inbound.Service.Submit", func() (string, error) {
		if err := s.inbound.Submit(ctx, orderID); err != nil {
			return "", err
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}
	if err := run.execute(2, "SUBMITTED → APPROVED", "inbound.Service.Approve", func() (string, error) {
		if err := s.inbound.Approve(ctx, orderID, operator); err != nil {
			return "", err
		}
		return orderNo, nil
	}); err != nil {
		return run.result, err
	}

	batchNo := demoBatchNo()
	if err := run.execute(3, "APPROVED → PUTAWAY", "inbound.Service.Receive", func() (string, error) {
		detail, err := s.inbound.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		if len(detail.Details) == 0 {
			return "", errcode.DemoDataMissing
		}
		if err := s.inbound.Receive(ctx, orderID, detail.Details[0].ID, &inbounddto.ReceiveReq{
			DetailID: detail.Details[0].ID, Qty: qty, BatchNo: batchNo,
		}, operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s / 批次 %s / %d 件", orderNo, batchNo, qty), nil
	}); err != nil {
		return run.result, err
	}

	if err := run.execute(4, "PUTAWAY → COMPLETED", "inbound.Service.Putaway", func() (string, error) {
		detail, err := s.inbound.Get(ctx, orderID)
		if err != nil {
			return "", err
		}
		var putawayTaskID int64
		var putawayTaskNo string
		var putawayQty int
		for _, task := range detail.Tasks {
			if task.TaskType == taskmodel.TaskPutaway && task.Status != taskmodel.TaskCompleted && task.DoneQty < task.TargetQty {
				putawayTaskID = task.ID
				putawayTaskNo = task.TaskNo
				putawayQty = task.TargetQty - task.DoneQty
				break
			}
		}
		if putawayTaskID == 0 {
			return "", errcode.DemoDataMissing
		}
		if err := s.inbound.Putaway(ctx, putawayTaskID, location.ID, putawayQty, operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s / 库位 %s / %d 件", putawayTaskNo, location.Code, putawayQty), nil
	}); err != nil {
		return run.result, err
	}

	result := run.finish(fmt.Sprintf("一键补货完成：已通过完整入库流程补充 %d 件库存", qty))
	result.TargetPath = "/inventory"
	result.TargetLabel = "查看库存"
	return result, nil
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
