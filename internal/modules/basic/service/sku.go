package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"

	"gorm.io/gorm"
)

// 货品和条码业务。

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
	has, err := s.stock.HasStockBySKU(ctx, id)
	if err != nil {
		return err
	}
	if has {
		return errcode.SKUHasStock
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

func (s *Service) GetSKUByCode(ctx context.Context, code string) (*model.SKU, error) {
	sku, err := s.repo.GetSKUByCode(ctx, s.tm.DB(), code)
	if err != nil {
		return nil, errcode.SKUNotFound
	}
	return sku, nil
}
