package repository

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/dbutil"
)

type Repository struct{}

func New() *Repository { return &Repository{} }

// ---------- 仓库 ----------

func (r *Repository) GetWarehouseByCode(ctx context.Context, db *gorm.DB, code string) (*model.Warehouse, error) {
	var w model.Warehouse
	err := db.WithContext(ctx).Where("code = ?", code).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) GetWarehouse(ctx context.Context, db *gorm.DB, id int64) (*model.Warehouse, error) {
	var w model.Warehouse
	if err := db.WithContext(ctx).First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) CreateWarehouse(ctx context.Context, db *gorm.DB, w *model.Warehouse) error {
	return db.WithContext(ctx).Create(w).Error
}

func (r *Repository) UpdateWarehouse(ctx context.Context, db *gorm.DB, id int64, name, remark string, status *int) error {
	updates := map[string]any{"name": name, "remark": remark}
	if status != nil {
		updates["status"] = *status
	}
	return db.WithContext(ctx).Model(&model.Warehouse{}).Where("id = ?", id).Updates(updates).Error
}

func (r *Repository) DeleteWarehouse(ctx context.Context, db *gorm.DB, id int64) error {
	return db.WithContext(ctx).Unscoped().Delete(&model.Warehouse{}, id).Error
}

func (r *Repository) CountLocationsByWarehouse(ctx context.Context, db *gorm.DB, warehouseID int64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&model.Location{}).Where("warehouse_id = ?", warehouseID).Count(&n).Error
	return n, err
}

func (r *Repository) ListWarehouses(ctx context.Context, db *gorm.DB, keyword string, page, size int) ([]*model.Warehouse, int64, error) {
	q := db.WithContext(ctx).Model(&model.Warehouse{})
	if keyword != "" {
		q = q.Where("code LIKE ? OR name LIKE ?", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Warehouse
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// ---------- 库位 ----------

func (r *Repository) GetLocationByCode(ctx context.Context, db *gorm.DB, warehouseID int64, code string) (*model.Location, error) {
	var l model.Location
	err := db.WithContext(ctx).Where("warehouse_id = ? AND code = ?", warehouseID, code).First(&l).Error
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repository) GetLocation(ctx context.Context, db *gorm.DB, id int64) (*model.Location, error) {
	var l model.Location
	if err := db.WithContext(ctx).First(&l, id).Error; err != nil {
		return nil, err
	}
	return &l, nil
}

func (r *Repository) CreateLocation(ctx context.Context, db *gorm.DB, l *model.Location) error {
	return db.WithContext(ctx).Create(l).Error
}

func (r *Repository) CreateLocationBatch(ctx context.Context, db *gorm.DB, list []*model.Location) error {
	return db.WithContext(ctx).CreateInBatches(list, 200).Error
}

func (r *Repository) UpdateLocation(ctx context.Context, db *gorm.DB, id int64, status int) error {
	return db.WithContext(ctx).Model(&model.Location{}).Where("id = ?", id).Update("status", status).Error
}

// UpdateLocationStatusInTx 业务事务内更新库位状态。
func (r *Repository) UpdateLocationStatusInTx(tx *gorm.DB, id int64, status int) error {
	return tx.Model(&model.Location{}).Where("id = ?", id).Update("status", status).Error
}

func (r *Repository) DeleteLocation(ctx context.Context, db *gorm.DB, id int64) error {
	return db.WithContext(ctx).Unscoped().Delete(&model.Location{}, id).Error
}

func (r *Repository) ListLocations(ctx context.Context, db *gorm.DB, warehouseID int64, keyword string, page, size int) ([]*model.Location, int64, error) {
	q := db.WithContext(ctx).Model(&model.Location{})
	if warehouseID > 0 {
		q = q.Where("warehouse_id = ?", warehouseID)
	}
	if keyword != "" {
		q = q.Where("code LIKE ?", "%"+dbutil.LikePattern(keyword)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Location
	err := q.Order("code").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// ListLocationCodes 查询指定仓库已存在的库位编码集合（批量生成幂等跳过用）。
func (r *Repository) ListLocationCodes(ctx context.Context, db *gorm.DB, warehouseID int64) (map[string]struct{}, error) {
	var codes []string
	err := db.WithContext(ctx).Model(&model.Location{}).Where("warehouse_id = ?", warehouseID).Pluck("code", &codes).Error
	if err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		set[c] = struct{}{}
	}
	return set, nil
}

// ---------- SKU ----------

func (r *Repository) GetSKUByCode(ctx context.Context, db *gorm.DB, code string) (*model.SKU, error) {
	var s model.SKU
	err := db.WithContext(ctx).Where("code = ?", code).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetSKUByBarcode(ctx context.Context, db *gorm.DB, barcode string) (*model.SKU, error) {
	var s model.SKU
	err := db.WithContext(ctx).Where("barcode = ?", barcode).First(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetSKU(ctx context.Context, db *gorm.DB, id int64) (*model.SKU, error) {
	var s model.SKU
	if err := db.WithContext(ctx).First(&s, id).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) CreateSKU(ctx context.Context, db *gorm.DB, s *model.SKU) error {
	return db.WithContext(ctx).Create(s).Error
}

func (r *Repository) UpdateSKU(ctx context.Context, db *gorm.DB, s *model.SKU) error {
	return db.WithContext(ctx).Model(&model.SKU{}).Where("id = ?", s.ID).Updates(map[string]any{
		"code": s.Code, "barcode": s.Barcode, "name": s.Name, "spec": s.Spec, "unit": s.Unit,
	}).Error
}

func (r *Repository) DeleteSKU(ctx context.Context, db *gorm.DB, id int64) error {
	return db.WithContext(ctx).Unscoped().Delete(&model.SKU{}, id).Error
}

func (r *Repository) ListSKUs(ctx context.Context, db *gorm.DB, keyword string, page, size int) ([]*model.SKU, int64, error) {
	q := db.WithContext(ctx).Model(&model.SKU{})
	if keyword != "" {
		q = q.Where("code LIKE ? OR name LIKE ? OR barcode LIKE ?", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.SKU
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}
