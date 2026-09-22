package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/quota"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

// 货品和条码业务。

func (s *Service) CreateSKU(ctx context.Context, req *dto.SKUReq) error {
	db := s.tm.DB()
	// 公开租户配额：防止访客脚本批量建货品撑爆数据库。
	if err := quota.Guard(ctx, db, &model.SKU{}, s.limits.MaxSKUs, 1, "货品"); err != nil {
		return err
	}
	if _, err := s.repo.GetSKUByCode(ctx, db, req.Code); err == nil {
		return errcode.SKUExist
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return s.repo.CreateSKU(ctx, db, &model.SKU{
		Code: req.Code, Barcode: req.Barcode, Name: req.Name, Spec: req.Spec, Unit: req.Unit, Status: 1,
	})
}

func (s *Service) UpdateSKU(ctx context.Context, id int64, req *dto.SKUReq) error {
	old, err := s.repo.GetSKU(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.SKUNotFound
		}
		return err
	}
	if err := s.repo.UpdateSKU(ctx, s.tm.DB(), &model.SKU{
		Base: sysmodel.Base{ID: id}, Code: req.Code, Barcode: req.Barcode, Name: req.Name, Spec: req.Spec, Unit: req.Unit,
	}); err != nil {
		return err
	}
	// 名称、规格等也会缓存；即使条码不变，更新成功后也需要失效。
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, barcodeKey(old.TenantID, old.Barcode), barcodeKey(old.TenantID, req.Barcode))
	}
	return nil
}

func (s *Service) DeleteSKU(ctx context.Context, id int64) error {
	var sku *model.SKU
	err := s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(txDB *gorm.DB) error {
		current, err := s.repo.GetSKUForUpdate(ctx, txDB, id)
		if err != nil {
			return err
		}
		has, err := s.stock.HasStockBySKU(ctx, txDB, id)
		if err != nil {
			return err
		}
		if has {
			return errcode.SKUHasStock
		}
		references, err := s.repo.CountSKUReferences(ctx, txDB, id)
		if err != nil {
			return err
		}
		if references > 0 {
			return errcode.SKUHasReferences
		}
		if err := s.repo.DeleteSKU(ctx, txDB, id); err != nil {
			return err
		}
		sku = current
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errcode.SKUNotFound
	}
	if err != nil {
		return err
	}
	if s.rdb != nil {
		_ = s.rdb.Del(ctx, barcodeKey(sku.TenantID, sku.Barcode))
	}
	return nil
}

func (s *Service) ListSKUs(ctx context.Context, q *dto.CommonQuery) ([]*model.SKU, int64, error) {
	return s.repo.ListSKUs(ctx, s.tm.DB(), q.Keyword, q.Page, q.PageSize)
}

func (s *Service) GetByBarcode(ctx context.Context, barcode string) (*model.SKU, error) {
	tenantID := tenant.FromContext(ctx)
	key := barcodeKey(tenantID, barcode)
	// 平台旁路查询没有唯一的租户范围，不使用条码缓存。
	useCache := s.rdb != nil && tenantID > 0
	if useCache {
		if val, err := s.rdb.Get(ctx, key); err == nil && val != "" {
			var cached model.SKU
			if err := json.Unmarshal([]byte(val), &cached); err == nil && cached.TenantID == tenantID && cached.Barcode == barcode {
				return &cached, nil
			}
		}
	}
	sku, err := s.repo.GetSKUByBarcode(ctx, s.tm.DB(), barcode)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.SKUNotFound
		}
		return nil, err
	}
	if useCache {
		if b, err := json.Marshal(sku); err == nil {
			_ = s.rdb.Set(ctx, key, string(b), time.Hour)
		}
	}
	return sku, nil
}

func barcodeKey(tenantID int64, barcode string) string {
	return fmt.Sprintf("gowms:tenant:%d:barcode:%s", tenantID, barcode)
}

func (s *Service) ValidateSKU(ctx context.Context, id int64) error {
	sku, err := s.repo.GetSKU(ctx, s.tm.DB(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.SKUNotFound
		}
		return err
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

func (s *Service) GetSKUByCode(ctx context.Context, code string) (*model.SKU, error) {
	sku, err := s.repo.GetSKUByCode(ctx, s.tm.DB(), code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.SKUNotFound
		}
		return nil, err
	}
	return sku, nil
}
