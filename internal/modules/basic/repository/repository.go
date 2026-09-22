package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/basic/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
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

// CountWarehouseReferences 统计仓库下的业务引用。库存单独通过 StockChecker
// 查询，便于返回更明确的错误；这里覆盖库位、任务和各类单据。
func (r *Repository) CountWarehouseReferences(ctx context.Context, db *gorm.DB, warehouseID int64) (int64, error) {
	return r.countReferences(ctx, db, warehouseID, []referenceColumn{
		{"wms_location", "warehouse_id"},
		{"wms_task", "warehouse_id"},
		{"wms_receipt_order", "warehouse_id"},
		{"wms_shipment_order", "warehouse_id"},
		{"wms_stocktake_order", "warehouse_id"},
	})
}

// CountLocationReferences 统计库位上的任务和单据引用。
func (r *Repository) CountLocationReferences(ctx context.Context, db *gorm.DB, locationID int64) (int64, error) {
	return r.countReferences(ctx, db, locationID, []referenceColumn{
		{"wms_task", "location_id"},
		{"wms_allocation", "location_id"},
		{"wms_stocktake_order", "location_id"},
		{"wms_stocktake_detail", "location_id"},
	})
}

// CountSKUReferences 统计货品上的任务、单据明细和分配引用。
func (r *Repository) CountSKUReferences(ctx context.Context, db *gorm.DB, skuID int64) (int64, error) {
	return r.countReferences(ctx, db, skuID, []referenceColumn{
		{"wms_task", "sku_id"},
		{"wms_receipt_order_detail", "sku_id"},
		{"wms_shipment_order_detail", "sku_id"},
		{"wms_allocation", "sku_id"},
		{"wms_stocktake_detail", "sku_id"},
	})
}

type referenceColumn struct {
	table  string
	column string
}

func (r *Repository) countReferences(ctx context.Context, db *gorm.DB, id int64, refs []referenceColumn) (int64, error) {
	for _, ref := range refs {
		// Unscoped 保留历史引用：即使业务单据已经软删除，也不能让基础资料被硬删后留下孤儿行。
		q := db.WithContext(ctx).Unscoped().Table(ref.table).Where(ref.column+" = ?", id)
		if tenantID, scoped := tenant.Scope(ctx); scoped {
			q = q.Where("tenant_id = ?", tenantID)
		}
		var count int64
		if err := q.Count(&count).Error; err != nil {
			return 0, err
		}
		if count > 0 {
			return count, nil
		}
	}
	return 0, nil
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
