package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/stocktake/model"
)

type Repository struct{}

func New() *Repository { return &Repository{} }

func (r *Repository) CreateOrder(tx *gorm.DB, order *model.StocktakeOrder, details []*model.StocktakeDetail) error {
	if err := tx.Create(order).Error; err != nil {
		return err
	}
	for _, d := range details {
		d.OrderID = order.ID
	}
	return tx.CreateInBatches(details, 100).Error
}

func (r *Repository) GetOrderForUpdate(tx *gorm.DB, id int64) (*model.StocktakeOrder, error) {
	var o model.StocktakeOrder
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, id).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) GetOrder(ctx context.Context, db *gorm.DB, id int64) (*model.StocktakeOrder, error) {
	var o model.StocktakeOrder
	if err := db.WithContext(ctx).First(&o, id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

// UpdateStatus 状态推进。
func (r *Repository) UpdateStatus(tx *gorm.DB, id int64, from, to model.OrderStatus) (int64, error) {
	res := tx.Model(&model.StocktakeOrder{}).Where("id = ? AND status = ?", id, from).Update("status", to)
	return res.RowsAffected, res.Error
}

func (r *Repository) ListOrders(ctx context.Context, db *gorm.DB, warehouseID int64, status string, page, size int) ([]*model.StocktakeOrder, int64, error) {
	q := db.WithContext(ctx).Model(&model.StocktakeOrder{})
	if warehouseID > 0 {
		q = q.Where("warehouse_id = ?", warehouseID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.StocktakeOrder
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

func (r *Repository) ListDetails(tx *gorm.DB, orderID int64) ([]*model.StocktakeDetail, error) {
	var list []*model.StocktakeDetail
	err := tx.Where("order_id = ?", orderID).Order("id").Find(&list).Error
	return list, err
}

func (r *Repository) GetDetail(tx *gorm.DB, detailID int64) (*model.StocktakeDetail, error) {
	var d model.StocktakeDetail
	err := tx.First(&d, detailID).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repository) UpdateDetailActual(tx *gorm.DB, detailID int64, actual int) error {
	return tx.Model(&model.StocktakeDetail{}).Where("id = ?", detailID).
		Update("actual_qty", actual).Error
}

// MarkAdjusted 写回差异与调整标记。
func (r *Repository) MarkAdjusted(tx *gorm.DB, detailID int64, diff int) error {
	return tx.Model(&model.StocktakeDetail{}).Where("id = ?", detailID).
		Updates(map[string]any{"diff_qty": diff, "adjusted": true}).Error
}

// SnapshotInventory 创建快照：按仓库/库位范围取账面库存行。
func (r *Repository) SnapshotInventory(tx *gorm.DB, warehouseID, locationID int64) ([]*model.StocktakeDetail, error) {
	q := tx.Table("wms_inventory i").
		Select("i.id AS inventory_id, i.sku_id, i.location_id, i.batch_no, i.stock_quantity AS book_qty").
		Joins("JOIN wms_sku s ON s.id = i.sku_id AND s.deleted_at IS NULL").
		Where("i.warehouse_id = ? AND i.deleted_at IS NULL AND i.stock_quantity > 0", warehouseID)
	if locationID > 0 {
		q = q.Where("i.location_id = ?", locationID)
	}
	type row struct {
		InventoryID int64
		SkuID       int64
		LocationID  int64
		BatchNo     string
		BookQty     int
	}
	var rows []row
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	// 填充 SKU 编码/名称与库位编码
	details := make([]*model.StocktakeDetail, 0, len(rows))
	if len(rows) == 0 {
		return details, nil
	}
	skuMap := map[int64][2]string{}
	var skus []struct {
		ID   int64
		Code string
		Name string
	}
	if err := tx.Table("wms_sku").Where("deleted_at IS NULL").Select("id, code, name").Scan(&skus).Error; err != nil {
		return nil, err
	}
	for _, s := range skus {
		skuMap[s.ID] = [2]string{s.Code, s.Name}
	}
	locMap := map[int64]string{}
	var locs []struct {
		ID   int64
		Code string
	}
	if err := tx.Table("wms_location").Where("deleted_at IS NULL").Select("id, code").Scan(&locs).Error; err != nil {
		return nil, err
	}
	for _, l := range locs {
		locMap[l.ID] = l.Code
	}
	for _, row := range rows {
		code, name := skuMap[row.SkuID][0], skuMap[row.SkuID][1]
		details = append(details, &model.StocktakeDetail{
			InventoryID: row.InventoryID, SKUID: row.SkuID, SKUCode: code, SKUName: name,
			LocationID: row.LocationID, LocationCode: locMap[row.LocationID],
			BatchNo: row.BatchNo, BookQty: row.BookQty,
		})
	}
	return details, nil
}
