package service

import (
	"context"
	"slices"
	"time"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

// ShipWaveOrder is the only wave path that consumes reserved inventory.
// Header -> tasks -> allocations -> inventory -> order/details; ordinary paths
// reject WaveID immediately after locking the task, without waiting for a header.
func (s *Service) ShipWaveOrder(ctx context.Context, id, memberID int64, operator, key string) (*dto.WaveAck, error) {
	ctx = waveContext(ctx)
	payload := struct{ WaveID, MemberID int64 }{id, memberID}
	return s.waveCommand(ctx, key, "ship", operator, payload, func(db *gorm.DB) (int64, error) {
		w, err := s.repo.Wave(db, id, true)
		if err != nil {
			return 0, waveError(err)
		}
		member, err := s.repo.WaveOrder(db, id, memberID)
		if err != nil {
			return 0, waveError(err)
		}
		if member.ShippedAt != nil {
			return id, nil
		}
		if w.Status != model.WaveSorting && w.Status != model.WaveReady {
			return 0, errcode.WaveStatusWrong
		}
		if err = waveOwner(w, operator); err != nil {
			return 0, err
		}
		items, err := s.repo.WaveOrderItems(db, memberID)
		if err != nil {
			return 0, err
		}
		if len(items) == 0 || len(items) > maxWaveTasks {
			return 0, errcode.WaveOrderInvalid
		}
		for _, item := range items {
			if item.SortedQty != item.TargetQty {
				return 0, errcode.WaveNotSorted
			}
		}
		tasks, err := s.taskAPI.ListForWave(ctx, db, []int64{member.OrderID}, maxWaveTasks+1, true)
		if err != nil {
			return 0, err
		}
		if len(tasks) != len(items) {
			return 0, errcode.TaskAllocationMismatch
		}
		byTask := make(map[int64]*taskmodel.Task, len(tasks))
		for _, task := range tasks {
			byTask[task.ID] = task
		}
		allocations, err := s.repo.WaveAllocations(db, []int64{member.OrderID}, true)
		if err != nil {
			return 0, err
		}
		if len(allocations) != len(items) {
			return 0, errcode.TaskAllocationMismatch
		}
		byAlloc := make(map[int64]*model.Allocation, len(allocations))
		for _, a := range allocations {
			byAlloc[a.ID] = a
		}
		var total int64
		taskNoByAllocation := make(map[int64]string, len(items))
		for _, item := range items {
			task := byTask[item.TaskID]
			a := byAlloc[item.AllocationID]
			if task == nil || a == nil || task.WaveID != id || task.DoneQty != 0 || task.Status != taskmodel.TaskCreated || a.Status != model.AllocAllocated || a.PickedQty != 0 || task.AllocationID != a.ID || task.DetailID != a.DetailID || a.OrderID != member.OrderID || task.SKUID != a.SKUID || task.TargetQty != item.TargetQty || a.AllocatedQty != item.TargetQty {
				return 0, errcode.TaskAllocationMismatch
			}
			total += int64(item.TargetQty)
			taskNoByAllocation[a.ID] = task.TaskNo
		}
		if total <= 0 || total > 2147483647 {
			return 0, errcode.QuantityLimitExceeded
		}
		// Inventory locks use a stable order even when tasks/allocation IDs differ.
		slices.SortFunc(allocations, func(a, b *model.Allocation) int {
			if a.InventoryID < b.InventoryID {
				return -1
			}
			if a.InventoryID > b.InventoryID {
				return 1
			}
			return 0
		})
		for _, a := range allocations {
			if err = s.inv.Ship(ctx, db, &invapi.ShipReq{InventoryID: a.InventoryID, Quantity: a.AllocatedQty, OrderNo: member.OrderNo, TaskNo: taskNoByAllocation[a.ID], Operator: operator}); err != nil {
				return 0, err
			}
		}
		for _, item := range items {
			task := byTask[item.TaskID]
			a := byAlloc[item.AllocationID]
			if err = s.taskAPI.AddProgress(ctx, db, task, item.TargetQty, operator); err != nil {
				return 0, err
			}
			n, updateErr := s.repo.IncrAllocationPicked(db, a, item.TargetQty, model.AllocPicked)
			if updateErr != nil {
				return 0, updateErr
			}
			if n != 1 {
				return 0, errcode.AllocConflict
			}
		}
		o, err := s.repo.GetOrderForUpdate(db, member.OrderID)
		if err != nil {
			return 0, err
		}
		if o.Status != model.OrderPicking || o.PickedQty != 0 || int64(o.AllocatedQty) != total || o.WarehouseID != w.WarehouseID {
			return 0, errcode.ShipOrderStatusWrong
		}
		n, err := s.repo.IncrOrderPicked(db, o.ID, int(total))
		if err != nil {
			return 0, err
		}
		if n != 1 {
			return 0, errcode.ShipOrderVersionBad
		}
		for _, item := range items {
			n, err = s.repo.IncrDetailPicked(db, byAlloc[item.AllocationID].DetailID, item.TargetQty)
			if err != nil {
				return 0, err
			}
			if n != 1 {
				return 0, errcode.TaskAllocationMismatch
			}
		}
		n, err = s.repo.ShipIfFullyPicked(db, o.ID)
		if err != nil {
			return 0, err
		}
		if n != 1 {
			return 0, errcode.ShipOrderStatusWrong
		}
		if err = s.repo.MarkWaveOrderShipped(db, memberID, time.Now()); err != nil {
			return 0, err
		}
		left, err := s.repo.WaveUnshipped(db, id)
		if err != nil {
			return 0, err
		}
		fields := map[string]any{"lease_expire_at": time.Now().Add(pickLeaseTTL)}
		if left == 0 {
			fields["status"] = model.WaveCompleted
		}
		if err = s.repo.UpdateWave(db, id, fields); err != nil {
			return 0, err
		}
		return id, nil
	})
}
