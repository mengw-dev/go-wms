package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

func (s *Service) CreateWave(ctx context.Context, req *dto.CreateWaveReq, operator, key string) (*dto.WaveAck, error) {
	if req == nil || len(req.OrderIDs) < 1 || len(req.OrderIDs) > maxWaveOrders || len([]rune(req.Remark)) > 255 {
		return nil, errcode.WaveBudgetExceeded
	}
	ids := slices.Clone(req.OrderIDs)
	slices.Sort(ids)
	for i, id := range ids {
		if id <= 0 || (i > 0 && ids[i-1] == id) {
			return nil, errcode.ParamError
		}
	}
	normalized := dto.CreateWaveReq{OrderIDs: ids, Remark: strings.TrimSpace(req.Remark)}
	ctx = waveContext(ctx)
	return s.waveCommand(ctx, key, "create", operator, normalized, func(db *gorm.DB) (int64, error) {
		// Tasks first, then allocations/orders: ordinary picking/cancel also starts at tasks.
		tasks, err := s.taskAPI.ListForWave(ctx, db, ids, maxWaveTasks+1, true)
		if err != nil {
			return 0, err
		}
		if len(tasks) > maxWaveTasks {
			return 0, errcode.WaveBudgetExceeded
		}
		if len(tasks) == 0 {
			return 0, errcode.WaveOrderInvalid
		}
		allocations, err := s.repo.WaveAllocations(db, ids, true)
		if err != nil {
			return 0, err
		}
		if len(allocations) != len(tasks) {
			return 0, errcode.WaveOrderInvalid
		}
		allocMap := make(map[int64]*model.Allocation, len(allocations))
		for _, a := range allocations {
			allocMap[a.ID] = a
		}
		qtyByOrder := make(map[int64]int64)
		skuIDs := make([]int64, 0, len(tasks))
		now := time.Now()
		for _, task := range tasks {
			if task.Status != taskmodel.TaskCreated || task.DoneQty != 0 || task.WaveID != 0 || task.TargetQty <= 0 || (task.LeaseExpireAt != nil && task.LeaseExpireAt.After(now)) {
				return 0, errcode.WaveOrderInvalid
			}
			a := allocMap[task.AllocationID]
			if a == nil || a.OrderID != task.OrderID || a.DetailID != task.DetailID || a.SKUID != task.SKUID || a.LocationID != task.LocationID || a.LocationCode != task.LocationCode || a.BatchNo != task.BatchNo || a.AllocatedQty != task.TargetQty || a.PickedQty != 0 || a.Status != model.AllocAllocated {
				return 0, errcode.TaskAllocationMismatch
			}
			qtyByOrder[task.OrderID] += int64(task.TargetQty)
			skuIDs = append(skuIDs, task.SKUID)
		}
		slices.Sort(skuIDs)
		skuIDs = slices.Compact(skuIDs)
		waveID := snowflake.Next()
		wave := &model.Wave{Base: modelbase.Base{ID: waveID}, TenantID: tenant.FromContext(ctx), WaveNo: fmt.Sprintf("BC%s%d", now.Format("20060102"), waveID), Status: model.WaveCreated, Remark: normalized.Remark, CreatedBy: operator}
		members := make([]*model.WaveOrder, 0, len(ids))
		memberMap := make(map[int64]*model.WaveOrder, len(ids))
		for i, id := range ids {
			o, readErr := s.repo.GetOrderForUpdate(db, id)
			if readErr != nil {
				return 0, waveError(readErr)
			}
			if i == 0 {
				wave.WarehouseID = o.WarehouseID
			}
			if o.WarehouseID != wave.WarehouseID || o.TenantID != wave.TenantID || o.Status != model.OrderPicking || o.PickedQty != 0 || o.ExpectedQty <= 0 || o.AllocatedQty != o.ExpectedQty || qtyByOrder[id] != int64(o.AllocatedQty) {
				return 0, errcode.WaveOrderInvalid
			}
			member := &model.WaveOrder{Base: modelbase.Base{ID: snowflake.Next()}, TenantID: wave.TenantID, WaveID: wave.ID, OrderID: id, ActiveOrderID: &id, OrderNo: o.OrderNo, SlotCode: fmt.Sprintf("S%02d", i+1)}
			members = append(members, member)
			memberMap[id] = member
		}
		if err := s.basic.ValidateWarehouseInTx(ctx, db, wave.WarehouseID); err != nil {
			return 0, err
		}
		// 与库存写入保持仓库→SKU引用锁顺序，不能先持SKU再等待仓库。
		skus, err := s.basic.GetSKUsByIDsForShare(ctx, db, skuIDs)
		if err != nil {
			return 0, err
		}
		lines := make([]*model.WaveLine, 0)
		byInventory := make(map[int64]*model.WaveLine)
		items := make([]*model.WaveItem, 0, len(tasks))
		taskIDs := make([]int64, 0, len(tasks))
		for _, task := range tasks {
			a := allocMap[task.AllocationID]
			if task.WarehouseID != wave.WarehouseID {
				return 0, errcode.TaskAllocationMismatch
			}
			sku := skus[task.SKUID]
			if sku == nil {
				return 0, errcode.SKUNotFound
			}
			line := byInventory[a.InventoryID]
			if line == nil {
				line = &model.WaveLine{Base: modelbase.Base{ID: snowflake.Next()}, TenantID: wave.TenantID, WaveID: wave.ID, InventoryID: a.InventoryID, SKUID: task.SKUID, SKUCode: sku.Code, Barcode: sku.Barcode, SKUName: sku.Name, LocationCode: task.LocationCode, BatchNo: task.BatchNo}
				byInventory[a.InventoryID] = line
				lines = append(lines, line)
			} else if line.SKUID != task.SKUID || line.LocationCode != task.LocationCode || line.BatchNo != task.BatchNo {
				return 0, errcode.TaskAllocationMismatch
			}
			line.TargetQty += int64(task.TargetQty)
			items = append(items, &model.WaveItem{Base: modelbase.Base{ID: snowflake.Next()}, TenantID: wave.TenantID, WaveID: wave.ID, WaveOrderID: memberMap[task.OrderID].ID, LineID: line.ID, TaskID: task.ID, AllocationID: a.ID, TargetQty: task.TargetQty})
			taskIDs = append(taskIDs, task.ID)
		}
		if err := s.repo.CreateWave(db, wave, members, lines, items); err != nil {
			return 0, err
		}
		if err := s.repo.SetTaskWave(db, taskIDs, 0, wave.ID); err != nil {
			return 0, err
		}
		return wave.ID, nil
	})
}
