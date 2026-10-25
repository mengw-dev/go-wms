package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/outbound/model"
)

func (r *Repository) WaveAllocations(db *gorm.DB, orderIDs []int64, lock bool) ([]*model.Allocation, error) {
	list := make([]*model.Allocation, 0)
	q := db.Where("order_id IN ? AND status <> ?", orderIDs, model.AllocCancelled).Order("id").Limit(1001)
	if lock {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	err := q.Find(&list).Error
	return list, err
}

func (r *Repository) CreateWave(db *gorm.DB, wave *model.Wave, orders []*model.WaveOrder, lines []*model.WaveLine, items []*model.WaveItem) error {
	if err := db.Create(wave).Error; err != nil {
		return err
	}
	if err := db.CreateInBatches(orders, 100).Error; err != nil {
		return err
	}
	if err := db.CreateInBatches(lines, 100).Error; err != nil {
		return err
	}
	return db.CreateInBatches(items, 100).Error
}

func (r *Repository) UpdateWave(db *gorm.DB, id int64, fields map[string]any) error {
	return db.Model(&model.Wave{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repository) WaveLine(db *gorm.DB, waveID, id int64) (*model.WaveLine, error) {
	var line model.WaveLine
	err := db.Where("wave_id = ?", waveID).First(&line, id).Error
	return &line, err
}

func (r *Repository) WaveItem(db *gorm.DB, waveID, id int64) (*model.WaveItem, error) {
	var item model.WaveItem
	err := db.Where("wave_id = ?", waveID).First(&item, id).Error
	return &item, err
}

func (r *Repository) WaveOrder(db *gorm.DB, waveID, id int64) (*model.WaveOrder, error) {
	var order model.WaveOrder
	err := db.Where("wave_id = ?", waveID).First(&order, id).Error
	return &order, err
}

func (r *Repository) IncrementWavePicked(db *gorm.DB, id int64, qty int) error {
	return db.Model(&model.WaveLine{}).Where("id = ?", id).Update("picked_qty", gorm.Expr("picked_qty + ?", qty)).Error
}

func (r *Repository) IncrementWaveSorted(db *gorm.DB, id int64, qty int) error {
	return db.Model(&model.WaveItem{}).Where("id = ?", id).Update("sorted_qty", gorm.Expr("sorted_qty + ?", qty)).Error
}

func (r *Repository) WaveUnpicked(db *gorm.DB, id int64) (int64, error) {
	var count int64
	err := db.Model(&model.WaveLine{}).Where("wave_id = ? AND picked_qty <> target_qty", id).Count(&count).Error
	return count, err
}

func (r *Repository) WaveUnsorted(db *gorm.DB, id int64) (int64, error) {
	var count int64
	err := db.Model(&model.WaveItem{}).Where("wave_id = ? AND sorted_qty <> target_qty", id).Count(&count).Error
	return count, err
}

func (r *Repository) WaveOrderItems(db *gorm.DB, id int64) ([]*model.WaveItem, error) {
	items := make([]*model.WaveItem, 0)
	err := db.Where("wave_order_id = ?", id).Order("allocation_id").Find(&items).Error
	return items, err
}

func (r *Repository) MarkWaveOrderShipped(db *gorm.DB, id int64, now time.Time) error {
	return db.Model(&model.WaveOrder{}).Where("id = ?", id).Update("shipped_at", now).Error
}

func (r *Repository) WaveUnshipped(db *gorm.DB, id int64) (int64, error) {
	var count int64
	err := db.Model(&model.WaveOrder{}).Where("wave_id = ? AND shipped_at IS NULL", id).Count(&count).Error
	return count, err
}

func (r *Repository) ReleaseWaveOrders(db *gorm.DB, id int64) error {
	return db.Model(&model.WaveOrder{}).Where("wave_id = ?", id).Update("active_order_id", nil).Error
}
