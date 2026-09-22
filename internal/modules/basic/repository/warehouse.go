package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/dbutil"
)

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

// GetWarehouseForUpdate 在同一事务内锁定仓库行，串行化删除与库位创建。
func (r *Repository) GetWarehouseForUpdate(ctx context.Context, db *gorm.DB, id int64) (*model.Warehouse, error) {
	var w model.Warehouse
	if err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *Repository) CreateWarehouse(ctx context.Context, db *gorm.DB, w *model.Warehouse) error {
	return db.WithContext(ctx).Create(w).Error
}

func (r *Repository) UpdateWarehouse(ctx context.Context, db *gorm.DB, id int64, name, remark string) error {
	return db.WithContext(ctx).Model(&model.Warehouse{}).Where("id = ?", id).
		Updates(map[string]any{"name": name, "remark": remark}).Error
}

// UpdateWarehouseStatus 只更新仓库状态，避免状态接口覆盖名称和备注。
func (r *Repository) UpdateWarehouseStatus(ctx context.Context, db *gorm.DB, id int64, status int) error {
	return db.WithContext(ctx).Model(&model.Warehouse{}).Where("id = ?", id).Update("status", status).Error
}

func (r *Repository) DeleteWarehouse(ctx context.Context, db *gorm.DB, id int64) error {
	res := db.WithContext(ctx).Unscoped().Delete(&model.Warehouse{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) CountLocationsByWarehouse(ctx context.Context, db *gorm.DB, warehouseID int64) (int64, error) {
	var n int64
	err := db.WithContext(ctx).Model(&model.Location{}).Where("warehouse_id = ?", warehouseID).Count(&n).Error
	return n, err
}

func (r *Repository) ListWarehouses(ctx context.Context, db *gorm.DB, keyword string, status *int, page, size int) ([]*model.Warehouse, int64, error) {
	q := db.WithContext(ctx).Model(&model.Warehouse{})
	if status != nil {
		q = q.Where("status = ?", *status)
	}
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
