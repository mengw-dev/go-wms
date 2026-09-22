package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/dbutil"
)

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

// GetLocationForUpdate 在同一事务内锁定库位行，串行化删除与库存创建。
func (r *Repository) GetLocationForUpdate(ctx context.Context, db *gorm.DB, id int64) (*model.Location, error) {
	var l model.Location
	if err := db.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).First(&l, id).Error; err != nil {
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
	res := db.WithContext(ctx).Unscoped().Delete(&model.Location{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
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
