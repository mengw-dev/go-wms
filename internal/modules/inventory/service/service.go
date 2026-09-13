package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
	pkgtx "gowms/internal/pkg/tx"
)

type Service struct {
	repo *repository.Repository
	tm   *pkgtx.Manager
}

func New(repo *repository.Repository, tm *pkgtx.Manager) *Service {
	return &Service{repo: repo, tm: tm}
}

// Increase 上架入库：stock += N, available += N。
// 四元组（仓库+库位+SKU+批次）存在则 FOR UPDATE 行锁后累加——锁内读到的是已提交最新值，
// 流水 Before/After 不再失真；不存在则创建，四元组唯一索引（uk_inv）兜底并发创建冲突；
// 并发创建产生死锁/冲突时由外层 TxRetry 或下方有限重试兜底。同事务写 RECEIVE 流水。
func (s *Service) Increase(ctx context.Context, tx *gorm.DB, req *api.IncreaseReq) error {
	if req.Quantity <= 0 {
		return errcode.ParamError
	}
	const tupleRetry = 3
	for i := 0; i < tupleRetry; i++ {
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

// Allocate FIFO 分配：available -= N, allocated += N（stock 不变，审核即锁库）。
// 双重防超卖：FOR UPDATE 行锁串行化 + WHERE available >= N 条件更新。
// 可用不足时返回 30201，message 携带需要/实际数量。
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

	result := &api.AllocateResult{Rows: make([]api.AllocateRow, 0, 4)}
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
		result.Rows = append(result.Rows, api.AllocateRow{
			InventoryID: inv.ID, LocationID: inv.LocationID,
			LocationCode: inv.LocationCode,
			BatchNo:      inv.BatchNo, Quantity: take,
		})
		remaining -= take
	}
	result.Total = req.Quantity
	return result, nil
}

// Ship 发货扣减：stock -= N, allocated -= N；WHERE 双条件防负 + 行锁读记录流水。
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

// Release 取消分配：available += N, allocated -= N。
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

// Adjust 盘点调整：以当前实时库存重算差异（行锁内），同事务写 ADJUST 流水。
func (s *Service) Adjust(ctx context.Context, tx *gorm.DB, req *api.AdjustReq) error {
	if req.NewStock < 0 {
		return errcode.AdjustNotAllow
	}
	inv, err := s.repo.GetForUpdate(tx, req.InventoryID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.InventoryNotFound
		}
		return err
	}
	delta := req.NewStock - inv.StockQuantity
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		if err := s.repo.AdjustPositive(tx, inv.ID, delta); err != nil {
			return err
		}
	} else {
		affected, err := s.repo.AdjustNegative(tx, inv.ID, -delta)
		if err != nil {
			return err
		}
		if affected == 0 { // 已分配库存不允许被盘点调减吃掉
			return errcode.AdjustNotAllow
		}
	}
	return s.repo.InsertTrans(tx, &model.InventoryTrans{
		ID:          snowflake.Next(),
		InventoryID: inv.ID, TransType: model.TransAdjust,
		QuantityChange: delta,
		BeforeQuantity: inv.StockQuantity, AfterQuantity: req.NewStock,
		AvailableBefore: inv.AvailableQty, AvailableAfter: inv.AvailableQty + delta,
		OrderNo: req.OrderNo, Operator: req.Operator,
	})
}

func availableNotEnoughMsg(skuID int64, need, actual int) string {
	return errcode.AvailableNotEnough.Msg + "：SKU[" + strconv.FormatInt(skuID, 10) + "] 需要" +
		strconv.Itoa(need) + "，实际可用" + strconv.Itoa(actual)
}

// ---------- 查询（Handler） ----------

func (s *Service) List(ctx context.Context, q *dto.InventoryQuery) ([]*model.Inventory, int64, error) {
	return s.repo.List(ctx, s.tm.DB(), &repository.QueryFilter{
		WarehouseID: q.WarehouseID, LocationID: q.LocationID, SKUID: q.SKUID,
		SKUKeyword: q.SKUKeyword, Page: q.Page, Size: q.PageSize,
	})
}

func (s *Service) SummaryBySKU(ctx context.Context, q *dto.SummaryQuery) ([]map[string]any, int64, error) {
	return s.repo.SummaryBySKU(ctx, s.tm.DB(), q.WarehouseID, q.Page, q.PageSize)
}

func (s *Service) ListTrans(ctx context.Context, q *dto.TransQuery) ([]*model.InventoryTrans, int64, error) {
	return s.repo.ListTrans(ctx, s.tm.DB(), q.InventoryID, q.OrderNo, q.TransType, q.Page, q.PageSize)
}

func (s *Service) HasStockByWarehouse(ctx context.Context, warehouseID int64) (bool, error) {
	return s.repo.HasStockByWarehouse(ctx, s.tm.DB(), warehouseID)
}

func (s *Service) HasStockByLocation(ctx context.Context, locationID int64) (bool, error) {
	return s.repo.HasStockByLocation(ctx, s.tm.DB(), locationID)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
