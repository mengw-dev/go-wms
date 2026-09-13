package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gowms/internal/modules/basic/api"
	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	"gowms/internal/modules/basic/repository"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/tx"

	"gorm.io/gorm"
)

type Service struct {
	repo  *repository.Repository
	tm    *tx.Manager
	rdb   redisClient
	stock api.StockChecker
}

func New(repo *repository.Repository, tm *tx.Manager, rdb redisClient, stock api.StockChecker) *Service {
	return &Service{repo: repo, tm: tm, rdb: rdb, stock: stock}
}

// redisClient 用接口避免直接依赖 go-redis 类型（nil 安全）。
type redisClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, val any, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

// ---------- 仓库 ----------

func (s *Service) CreateWarehouse(ctx context.Context, req *dto.WarehouseReq) error {
	if _, err := s.repo.GetWarehouseByCode(ctx, s.tm.DB(), req.Code); err == nil {
		return errcode.WarehouseExist
	}
	return s.repo.CreateWarehouse(ctx, s.tm.DB(), &model.Warehouse{
		Code: req.Code, Name: req.Name, Remark: req.Remark, Status: 1,
	})
}

func (s *Service) UpdateWarehouse(ctx context.Context, id int64, req *dto.WarehouseReq, status *int) error {
	return s.repo.UpdateWarehouse(ctx, s.tm.DB(), id, req.Name, req.Remark, status)
}

// DeleteWarehouse 存在库存的仓库禁止删除。
func (s *Service) DeleteWarehouse(ctx context.Context, id int64) error {
	has, err := s.stock.HasStockByWarehouse(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return errcode.WarehouseHasStock
	}
	return s.repo.DeleteWarehouse(ctx, s.tm.DB(), id)
}

func (s *Service) ListWarehouses(ctx context.Context, q *dto.WarehouseQuery) ([]*model.Warehouse, int64, error) {
	return s.repo.ListWarehouses(ctx, s.tm.DB(), q.Keyword, q.Page, q.PageSize)
}

// ---------- 库位 ----------

// BatchCreateLocations 批量初始化库位：编码 {zone}-{row}-{col}，已存在跳过（幂等）。
func (s *Service) BatchCreateLocations(ctx context.Context, req *dto.LocationBatchReq) (created int, err error) {
	if req.RowTo < req.RowFrom || req.ColTo < req.ColFrom {
		return 0, errcode.ParamError
	}
	if (req.RowTo-req.RowFrom+1)*(req.ColTo-req.ColFrom+1) > 1000 {
		return 0, errcode.New(20013, "单次批量生成不超过 1000 个库位")
	}
	exists, err := s.repo.ListLocationCodes(ctx, s.tm.DB(), req.WarehouseID)
	if err != nil {
		return 0, err
	}
	list := make([]*model.Location, 0, 64)
	for row := req.RowFrom; row <= req.RowTo; row++ {
		for col := req.ColFrom; col <= req.ColTo; col++ {
			code := fmt.Sprintf("%s-%02d-%02d", req.Zone, row, col)
			if _, dup := exists[code]; dup {
				continue
			}
			list = append(list, &model.Location{
				WarehouseID: req.WarehouseID, Code: code, Zone: req.Zone,
				Status: model.LocationStatusIdle,
			})
		}
	}
	if len(list) == 0 {
		return 0, nil
	}
	if err := s.repo.CreateLocationBatch(ctx, s.tm.DB(), list); err != nil {
		return 0, err
	}
	return len(list), nil
}

// DeleteLocation 有库存只能禁用不能删除。
func (s *Service) DeleteLocation(ctx context.Context, id int64) error {
	has, err := s.stock.HasStockByLocation(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return errcode.LocationHasStock
	}
	return s.repo.DeleteLocation(ctx, s.tm.DB(), id)
}

func (s *Service) UpdateLocationStatus(ctx context.Context, id int64, status int) error {
	if status != model.LocationStatusIdle && status != model.LocationStatusDisabled && status != model.LocationStatusOccupied {
		return errcode.ParamError
	}
	return s.repo.UpdateLocation(ctx, s.tm.DB(), id, status)
}

func (s *Service) ListLocations(ctx context.Context, q *dto.LocationQuery) ([]*model.Location, int64, error) {
	return s.repo.ListLocations(ctx, s.tm.DB(), q.WarehouseID, q.Keyword, q.Page, q.PageSize)
}

// ---------- SKU ----------

func (s *Service) CreateSKU(ctx context.Context, req *dto.SKUReq) error {
	db := s.tm.DB()
	if _, err := s.repo.GetSKUByCode(ctx, db, req.Code); err == nil {
		return errcode.SKUExist
	}
	return s.repo.CreateSKU(ctx, db, &model.SKU{
		Code: req.Code, Barcode: req.Barcode, Name: req.Name, Spec: req.Spec, Unit: req.Unit, Status: 1,
	})
}

func (s *Service) UpdateSKU(ctx context.Context, id int64, req *dto.SKUReq) error {
	old, err := s.repo.GetSKU(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.SKUNotFound
	}
	if err := s.repo.UpdateSKU(ctx, s.tm.DB(), &model.SKU{
		Base: sysmodel.Base{ID: id}, Code: req.Code, Barcode: req.Barcode, Name: req.Name, Spec: req.Spec, Unit: req.Unit,
	}); err != nil {
		return err
	}
	if old.Barcode != req.Barcode { // 条码变更失效缓存
		_ = s.rdb.Del(ctx, barcodeKey(old.Barcode), barcodeKey(req.Barcode))
	}
	return nil
}

func (s *Service) DeleteSKU(ctx context.Context, id int64) error {
	sku, err := s.repo.GetSKU(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.SKUNotFound
	}
	if err := s.repo.DeleteSKU(ctx, s.tm.DB(), id); err != nil {
		return err
	}
	_ = s.rdb.Del(ctx, barcodeKey(sku.Barcode))
	return nil
}

func (s *Service) ListSKUs(ctx context.Context, q *dto.CommonQuery) ([]*model.SKU, int64, error) {
	return s.repo.ListSKUs(ctx, s.tm.DB(), q.Keyword, q.Page, q.PageSize)
}

// GetByBarcode 扫码反查：Redis 缓存（<50ms），未命中回源并写缓存；变更/删除时主动失效。
func (s *Service) GetByBarcode(ctx context.Context, barcode string) (*model.SKU, error) {
	key := barcodeKey(barcode)
	if s.rdb != nil {
		if val, err := s.rdb.Get(ctx, key); err == nil && val != "" {
			var cached model.SKU
			if err := json.Unmarshal([]byte(val), &cached); err == nil {
				return &cached, nil
			}
		}
	}
	sku, err := s.repo.GetSKUByBarcode(ctx, s.tm.DB(), barcode)
	if err != nil {
		return nil, errcode.SKUNotFound
	}
	if s.rdb != nil {
		if b, err := json.Marshal(sku); err == nil {
			_ = s.rdb.Set(ctx, key, string(b), time.Hour)
		}
	}
	return sku, nil
}

func barcodeKey(barcode string) string {
	return "gowms:barcode:" + barcode
}

// ---------- BasicAPI 实现（跨模块） ----------

func (s *Service) ValidateWarehouse(ctx context.Context, id int64) error {
	w, err := s.repo.GetWarehouse(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.WarehouseNotFound
	}
	if w.Status != 1 {
		return errcode.WarehouseDisabled
	}
	return nil
}

func (s *Service) ValidateLocation(ctx context.Context, id int64) error {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.LocationNotFound
	}
	if l.Status == model.LocationStatusDisabled {
		return errcode.LocationDisabled
	}
	return nil
}

// ValidateLocationInWarehouse 在 ValidateLocation 基础上增加归属校验：
// 库位必须属于指定仓库，防止把 A 仓库的单据上架到 B 仓库库位，产生跨仓库存记录。
func (s *Service) ValidateLocationInWarehouse(ctx context.Context, warehouseID, id int64) error {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.LocationNotFound
	}
	if l.Status == model.LocationStatusDisabled {
		return errcode.LocationDisabled
	}
	if l.WarehouseID != warehouseID {
		return errcode.LocationWarehouseMismatch
	}
	return nil
}

func (s *Service) ValidateSKU(ctx context.Context, id int64) error {
	sku, err := s.repo.GetSKU(ctx, s.tm.DB(), id)
	if err != nil {
		return errcode.SKUNotFound
	}
	if sku.Status != 1 {
		return errcode.SKUDisabled
	}
	return nil
}

func (s *Service) GetSKU(ctx context.Context, id int64) (*model.SKU, error) {
	sku, err := s.repo.GetSKU(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.SKUNotFound
		}
		return nil, err
	}
	return sku, nil
}

func (s *Service) GetLocation(ctx context.Context, id int64) (*model.Location, error) {
	l, err := s.repo.GetLocation(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.LocationNotFound
		}
		return nil, err
	}
	return l, nil
}

func (s *Service) GetWarehouseByCode(ctx context.Context, code string) (*model.Warehouse, error) {
	w, err := s.repo.GetWarehouseByCode(ctx, s.tm.DB(), code)
	if err != nil {
		return nil, errcode.WarehouseNotFound
	}
	return w, nil
}

func (s *Service) GetSKUByCode(ctx context.Context, code string) (*model.SKU, error) {
	sku, err := s.repo.GetSKUByCode(ctx, s.tm.DB(), code)
	if err != nil {
		return nil, errcode.SKUNotFound
	}
	return sku, nil
}

// UpdateLocationStatusInTx 业务事务内把库位标记为占用（上架入库时调用）。
func (s *Service) UpdateLocationStatusInTx(ctx context.Context, tx *gorm.DB, id int64, status int) error {
	if err := s.repo.UpdateLocationStatusInTx(tx, id, status); err != nil {
		return err
	}
	log.WithContext(ctx).Debug("location status updated", "location_id", id, "status", status)
	return nil
}
