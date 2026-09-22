package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/dbutil"
)

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

// GetSKUForUpdate 在同一事务内锁定 SKU 行，串行化删除与库存创建。
func (r *Repository) GetSKUForUpdate(ctx context.Context, db *gorm.DB, id int64) (*model.SKU, error) {
	var s model.SKU
	if err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, id).Error; err != nil {
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
	res := db.WithContext(ctx).Unscoped().Delete(&model.SKU{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
