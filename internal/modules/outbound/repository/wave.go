package repository

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/dbutil"
)

func (r *Repository) Wave(db *gorm.DB, id int64, lock bool) (*model.Wave, error) {
	q := db
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var wave model.Wave
	err := q.First(&wave, id).Error
	return &wave, err
}

func (r *Repository) ListWaves(db *gorm.DB, warehouseID int64, keyword string, page, size int) ([]*model.Wave, int64, error) {
	q := db.Model(&model.Wave{})
	if warehouseID > 0 {
		q = q.Where("warehouse_id = ?", warehouseID)
	}
	if keyword != "" {
		q = q.Where("wave_no LIKE ?", "%"+dbutil.LikePattern(keyword)+"%")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	list := make([]*model.Wave, 0)
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// WaveCandidates is an advisory preview; creation rechecks tasks and orders under locks.
func (r *Repository) WaveCandidates(db *gorm.DB, tenantID, warehouseID int64, keyword string, limit int) ([]*model.ShipmentOrder, error) {
	q := db.Model(&model.ShipmentOrder{}).
		Where("warehouse_id = ? AND status = ? AND picked_qty = 0 AND allocated_qty = expected_qty AND expected_qty > 0", warehouseID, model.OrderPicking).
		Where(`EXISTS (SELECT 1 FROM wms_task t WHERE t.order_id = wms_shipment_order.id AND t.tenant_id = ? AND t.task_type = 'PICK' AND t.status <> 'CANCELLED' AND t.deleted_at IS NULL)`, tenantID).
		Where(`NOT EXISTS (SELECT 1 FROM wms_task t WHERE t.order_id = wms_shipment_order.id AND t.tenant_id = ? AND t.status <> 'CANCELLED' AND t.deleted_at IS NULL AND
		(t.wave_id <> 0 OR t.done_qty <> 0 OR t.status <> 'CREATED' OR t.lease_expire_at > CURRENT_TIMESTAMP(3)))`, tenantID)
	if keyword != "" {
		q = q.Where("order_no LIKE ? OR biz_order_no LIKE ?", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%")
	}
	list := make([]*model.ShipmentOrder, 0)
	err := q.Order("created_at,id").Limit(limit).Find(&list).Error
	return list, err
}

func (r *Repository) WaveParts(db *gorm.DB, id int64) ([]*model.WaveOrder, []*model.WaveLine, []*model.WaveItem, error) {
	orders := make([]*model.WaveOrder, 0)
	lines := make([]*model.WaveLine, 0)
	items := make([]*model.WaveItem, 0)
	if err := db.Where("wave_id = ?", id).Order("slot_code,id").Find(&orders).Error; err != nil {
		return nil, nil, nil, err
	}
	if err := db.Where("wave_id = ?", id).Order("location_code,sku_code,batch_no,id").Find(&lines).Error; err != nil {
		return nil, nil, nil, err
	}
	if err := db.Where("wave_id = ?", id).Order("id").Find(&items).Error; err != nil {
		return nil, nil, nil, err
	}
	return orders, lines, items, nil
}

func (r *Repository) SetTaskWave(db *gorm.DB, ids []int64, from, to int64) error {
	res := db.Model(&taskmodel.Task{}).Where("id IN ? AND wave_id = ?", ids, from).Update("wave_id", to)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != int64(len(ids)) {
		return gorm.ErrInvalidData
	}
	return nil
}
