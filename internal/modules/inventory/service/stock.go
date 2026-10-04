package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
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
			Base:        modelbase.Base{ID: snowflake.Next()},
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

// 分批分配参数：批大小决定单次候选查询的行数；
// 尝试次数给事务长度设上界，用尽说明当前事务的一致性快照已经过期，
// 交回外层 TxRetry 换新快照重试，比在旧快照上继续翻页更划算。
const (
	allocateBatchSize   = 20
	maxAllocateAttempts = 5
)

func (s *Service) Allocate(ctx context.Context, tx *gorm.DB, req *api.AllocateReq) (*api.AllocateResult, error) {
	if req.Quantity <= 0 {
		return nil, errcode.ParamError
	}

	allocation := &api.AllocateResult{Rows: make([]api.AllocateRow, 0, 4)}
	remaining := req.Quantity
	var pending []repository.FIFOCandidate // 当前批中尚未处理的候选
	var afterStockInTime time.Time
	var afterID int64
	candidatesExhausted := false

	for attempt := 0; attempt < maxAllocateAttempts && remaining > 0; attempt++ {
		if len(pending) == 0 {
			batch, err := s.repo.ListFIFOCandidates(tx, req.WarehouseID, req.SKUID, allocateBatchSize, afterStockInTime, afterID)
			if err != nil {
				return nil, err
			}
			if len(batch) == 0 {
				candidatesExhausted = true
				break
			}
			pending = batch
			last := batch[len(batch)-1]
			afterStockInTime, afterID = last.StockInTime, last.ID
		}

		// 只锁可能满足本次需求的前缀：快照可用量累加到覆盖剩余需求为止，
		// 剩下的候选留在 pending，避免因为前缀不够而跳过后面的候选。
		prefixLen := minRequiredPrefix(pending, remaining)
		prefix := pending[:prefixLen]
		pending = pending[prefixLen:]

		ids := make([]int64, 0, len(prefix))
		for _, c := range prefix {
			ids = append(ids, c.ID)
		}

		rows, err := s.repo.LockInventoryByIDs(tx, ids)
		if err != nil {
			return nil, err
		}
		// 加锁顺序由主键决定；取用顺序在锁内按 FIFO 重排。
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].StockInTime.Equal(rows[j].StockInTime) {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].StockInTime.Before(rows[j].StockInTime)
		})

		for _, inv := range rows {
			// 行已加锁，这里的可用量是最新已提交值：快照里显示有货的行可能已被并发扣空。
			if inv.AvailableQty <= 0 {
				continue
			}
			take := min(remaining, inv.AvailableQty)
			affected, err := s.repo.AllocateQty(tx, inv.ID, take)
			if err != nil {
				return nil, err
			}
			if affected == 0 { // 行已锁，理论上不会发生；命中则说明有未走行锁的写入，防御性回滚
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
				BatchNo: inv.BatchNo, Quantity: take,
			})
			remaining -= take
			if remaining == 0 {
				break
			}
		}
	}

	if remaining > 0 {
		// 轮次用尽但候选还没取完：结论不确定（可能只是快照过旧导致白跑），
		// 交给外层开新事务换一份新快照重试，不能在这里误报"库存不足"。
		if !candidatesExhausted {
			return nil, errcode.Conflict
		}
		// 候选确实取完了，这时快照与真实值一致，按它给出准确的可用量提示。
		totalAvailable, err := s.repo.SumAvailableQty(tx, req.WarehouseID, req.SKUID)
		if err != nil {
			return nil, err
		}
		return nil, errcode.New(errcode.AvailableNotEnough.Code,
			availableNotEnoughMsg(req.SKUID, req.Quantity, totalAvailable))
	}

	// 库位编码单独补：锁定读里不再 JOIN wms_location，避免把库位行一起锁住。
	locationIDs := make([]int64, 0, len(allocation.Rows))
	for _, row := range allocation.Rows {
		locationIDs = append(locationIDs, row.LocationID)
	}
	codes, err := s.repo.ListLocationCodes(tx, locationIDs)
	if err != nil {
		return nil, err
	}
	for i := range allocation.Rows {
		allocation.Rows[i].LocationCode = codes[allocation.Rows[i].LocationID]
	}

	allocation.Total = req.Quantity
	return allocation, nil
}

// minRequiredPrefix 返回按 FIFO 累加刚好覆盖需求所需的候选前缀长度（至少 1 个）。
// 只用于估算"该锁哪几行"，锁到手后仍以实际可用量为准。
func minRequiredPrefix(candidates []repository.FIFOCandidate, required int) int {
	sum := 0
	for i, c := range candidates {
		sum += c.AvailableQty
		if sum >= required {
			return i + 1
		}
	}
	return len(candidates)
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
