package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
	pkgtx "gowms/internal/pkg/tx"
)

// 库存变动：入库、分配、发货、释放和盘点调整。
//
// 三数量不变量：stock_quantity = available_quantity + allocated_quantity。
// 分配只在 available 与 allocated 之间移动；发货同时扣减 stock 和 allocated；
// 释放把 allocated 退回 available；盘点调减不能吃掉已分配库存。

func (s *Service) Increase(ctx context.Context, tx *gorm.DB, req *api.IncreaseReq) error {
	if req.Quantity <= 0 {
		return errcode.ParamError
	}
	if err := s.repo.LockBasicReferences(tx, req.WarehouseID, req.LocationID, req.SKUID); err != nil {
		return err
	}
	const tupleCreateRetries = 3
	for attempt := 0; attempt < tupleCreateRetries; attempt++ {
		// 行锁读取：并发上架在 FOR UPDATE 上串行化
		inv, err := s.repo.GetByTupleForUpdate(tx, req.WarehouseID, req.LocationID, req.SKUID, req.BatchNo)
		if err == nil {
			if err := s.repo.IncreaseQty(tx, inv.ID, req.Quantity, req.Quantity); err != nil {
				return err
			}
			return s.repo.InsertTrans(tx, &model.InventoryTrans{
				ID:          snowflake.Next(),
				InventoryID: inv.ID, TransType: model.TransReceive,
				QuantityChange: req.Quantity,
				BeforeQuantity: inv.StockQuantity, AfterQuantity: inv.StockQuantity + req.Quantity,
				AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty + req.Quantity,
				OrderNo: req.OrderNo, TaskNo: req.TaskNo, Operator: req.Operator,
			})
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 不存在 → 创建；唯一索引兜底并发创建
		inv = &model.Inventory{
			Base:        sysmodel.Base{ID: snowflake.Next()},
			WarehouseID: req.WarehouseID, LocationID: req.LocationID,
			SKUID: req.SKUID, BatchNo: req.BatchNo,
			StockQuantity: req.Quantity, AvailableQty: req.Quantity, AllocatedQty: 0,
			StockInTime: time.Now(), // FIFO 依据
		}
		err = s.repo.Create(tx, inv)
		if err == nil {
			return s.repo.InsertTrans(tx, &model.InventoryTrans{
				ID:          snowflake.Next(),
				InventoryID: inv.ID, TransType: model.TransReceive,
				QuantityChange: req.Quantity, BeforeQuantity: 0, AfterQuantity: req.Quantity,
				AvailableBefore: 0, AvailableAfter: req.Quantity,
				OrderNo: req.OrderNo, TaskNo: req.TaskNo, Operator: req.Operator,
			})
		}
		if !pkgtx.IsDuplicateErr(err) {
			return err // 死锁等错误交由外层 TxRetry 整事务重试
		}
		// 并发创建冲突：对方事务提交后重走锁读分支（未提交时 INSERT 已等待其提交才报重复）
	}
	return errcode.Conflict
}

func (s *Service) Allocate(ctx context.Context, tx *gorm.DB, req *api.AllocateReq) (*api.AllocateResult, error) {
	if req.Quantity <= 0 {
		return nil, errcode.ParamError
	}
	rows, err := s.repo.FindFIFOForUpdate(tx, req.WarehouseID, req.SKUID)
	if err != nil {
		return nil, err
	}
	totalAvailable := 0
	for _, inv := range rows {
		totalAvailable += inv.AvailableQty
	}
	if totalAvailable < req.Quantity {
		return nil, errcode.New(errcode.AvailableNotEnough.Code,
			availableNotEnoughMsg(req.SKUID, req.Quantity, totalAvailable))
	}

	allocation := &api.AllocateResult{Rows: make([]api.AllocateRow, 0, 4)}
	remaining := req.Quantity
	for _, inv := range rows {
		if remaining <= 0 {
			break
		}
		take := min(remaining, inv.AvailableQty)
		affected, err := s.repo.AllocateQty(tx, inv.ID, take)
		if err != nil {
			return nil, err
		}
		if affected == 0 { // 理论上行锁内不会发生；命中则说明有未走行锁的写入，防御性回滚
			return nil, errcode.Conflict
		}
		if err := s.repo.InsertTrans(tx, &model.InventoryTrans{
			ID: snowflake.Next(), InventoryID: inv.ID, TransType: model.TransAllocate,
			QuantityChange: 0, BeforeQuantity: inv.StockQuantity, AfterQuantity: inv.StockQuantity,
			AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty - take,
			OrderNo: req.OrderNo, Operator: req.Operator,
		}); err != nil {
			return nil, err
		}
		allocation.Rows = append(allocation.Rows, api.AllocateRow{
			InventoryID: inv.ID, LocationID: inv.LocationID,
			LocationCode: inv.LocationCode,
			BatchNo:      inv.BatchNo, Quantity: take,
		})
		remaining -= take
	}
	allocation.Total = req.Quantity
	return allocation, nil
}

func (s *Service) Ship(ctx context.Context, tx *gorm.DB, req *api.ShipReq) error {
	if req.Quantity <= 0 {
		return errcode.ParamError
	}
	inv, err := s.repo.GetForUpdate(tx, req.InventoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.InventoryNotFound
		}
		return err
	}
	affected, err := s.repo.ShipQty(tx, inv.ID, req.Quantity)
	if err != nil {
		return err
	}
	if affected == 0 {
		return errcode.ShipConflict
	}
	return s.repo.InsertTrans(tx, &model.InventoryTrans{
		ID:          snowflake.Next(),
		InventoryID: inv.ID, TransType: model.TransShip,
		QuantityChange: -req.Quantity,
		BeforeQuantity: inv.StockQuantity, AfterQuantity: inv.StockQuantity - req.Quantity,
		AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty,
		OrderNo: req.OrderNo, TaskNo: req.TaskNo, Operator: req.Operator,
	})
}

func (s *Service) Release(ctx context.Context, tx *gorm.DB, req *api.ReleaseReq) error {
	if req.Quantity <= 0 {
		return errcode.ParamError
	}
	inv, err := s.repo.GetForUpdate(tx, req.InventoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.InventoryNotFound
		}
		return err
	}
	affected, err := s.repo.ReleaseQty(tx, inv.ID, req.Quantity)
	if err != nil {
		return err
	}
	if affected == 0 {
		return errcode.AllocatedNotEnough
	}
	return s.repo.InsertTrans(tx, &model.InventoryTrans{
		ID:          snowflake.Next(),
		InventoryID: inv.ID, TransType: model.TransRelease,
		QuantityChange: 0,
		BeforeQuantity: inv.StockQuantity, AfterQuantity: inv.StockQuantity,
		AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty + req.Quantity,
		OrderNo: req.OrderNo, Operator: req.Operator,
	})
}

func (s *Service) Adjust(ctx context.Context, tx *gorm.DB, req *api.AdjustReq) (int, error) {
	if req.NewStock < 0 {
		return 0, errcode.AdjustNotAllow
	}
	inv, err := s.repo.GetForUpdate(tx, req.InventoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, errcode.InventoryNotFound
		}
		return 0, err
	}
	delta := req.NewStock - inv.StockQuantity
	if delta == 0 {
		return 0, nil
	}
	if delta > 0 {
		if err := s.repo.AdjustPositive(tx, inv.ID, delta); err != nil {
			return 0, err
		}
	} else {
		affected, err := s.repo.AdjustNegative(tx, inv.ID, -delta)
		if err != nil {
			return 0, err
		}
		if affected == 0 { // 已分配库存不允许被盘点调减吃掉
			return 0, errcode.AdjustNotAllow
		}
	}
	err = s.repo.InsertTrans(tx, &model.InventoryTrans{
		ID:          snowflake.Next(),
		InventoryID: inv.ID, TransType: model.TransAdjust,
		QuantityChange: delta,
		BeforeQuantity: inv.StockQuantity, AfterQuantity: req.NewStock,
		AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty + delta,
		OrderNo: req.OrderNo, Operator: req.Operator,
	})
	if err != nil {
		return 0, err
	}
	return delta, nil
}

func availableNotEnoughMsg(skuID int64, need, actual int) string {
	return errcode.AvailableNotEnough.Msg + "：SKU[" + strconv.FormatInt(skuID, 10) + "] 需要" +
		strconv.Itoa(need) + "，实际可用" + strconv.Itoa(actual)
}
