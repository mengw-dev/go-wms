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

// GetSKUsByIDs 批量查询 SKU，返回 id → SKU 映射。避免在循环中逐条 GetSKU 产生 N+1 查询。
func (r *Repository) GetSKUsByIDs(ctx context.Context, db *gorm.DB, ids []int64) (map[int64]*model.SKU, error) {
	if len(ids) == 0 {
		return map[int64]*model.SKU{}, nil
	}
	var list []*model.SKU
	if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&list).Error; err != nil {
		return nil, err
	}
	m := make(map[int64]*model.SKU, len(list))
	for _, s := range list {
		m[s.ID] = s
	}
	return m, nil
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
	res := db.WithContext(ctx).Model(&model.SKU{}).Where("id = ?", s.ID).Updates(map[string]any{
		"code": s.Code, "barcode": s.Barcode, "name": s.Name, "spec": s.Spec, "unit": s.Unit,
	})
	return dbutil.RequireAffected(res)
}

func (r *Repository) DeleteSKU(ctx context.Context, db *gorm.DB, id int64) error {
	return dbutil.RequireAffected(db.WithContext(ctx).Unscoped().Delete(&model.SKU{}, id))
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
