package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
)

type Repository struct{}

func New() *Repository { return &Repository{} }

// LockBasicReferences 按固定顺序锁定库存依赖的仓库、库位和 SKU。
// 基础资料删除使用同一行锁协议，避免“删除校验通过后又创建库存”的并发窗口。
func (r *Repository) LockBasicReferences(tx *gorm.DB, warehouseID, locationID, skuID int64) error {
	var warehouse basicmodel.Warehouse
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&warehouse, warehouseID).Error; err != nil {
		return err
	}
	var location basicmodel.Location
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&location, locationID).Error; err != nil {
		return err
	}
	var sku basicmodel.SKU
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sku, skuID).Error; err != nil {
		return err
	}
	return nil
}

// FindFIFOForUpdate 悲观行锁 + FIFO：锁定指定仓库/SKU 下所有可分配库存行，并联查库位编码。
// 防超卖第一层：FOR UPDATE 行锁串行化同一库存行的并发分配。
func (r *Repository) FindFIFOForUpdate(tx *gorm.DB, warehouseID, skuID int64) ([]*model.Inventory, error) {
	var list []*model.Inventory
	q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Table("wms_inventory i").
		Select("i.*, l.code AS location_code").
		Joins("JOIN wms_location l ON l.id = i.location_id AND l.tenant_id = i.tenant_id AND l.deleted_at IS NULL").
		Where("i.warehouse_id = ? AND i.sku_id = ? AND i.available_quantity > 0 AND i.deleted_at IS NULL", warehouseID, skuID)
	if tenantID, scoped := tenant.Scope(tx.Statement.Context); scoped {
		q = q.Where("i.tenant_id = ?", tenantID)
	}
	err := q.Order("i.stock_in_time ASC, i.id ASC").Scan(&list).Error
	return list, err
}

// GetForUpdate 按主键锁定库存行。
func (r *Repository) GetForUpdate(tx *gorm.DB, id int64) (*model.Inventory, error) {
	var inv model.Inventory
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&inv, id).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

// GetByTupleForUpdate 按四元组（仓库+库位+SKU+批次）锁定库存行（FOR UPDATE）。
// 行锁保证并发上架时读到的是最新值，流水 Before/After 不失真。
func (r *Repository) GetByTupleForUpdate(tx *gorm.DB, warehouseID, locationID, skuID int64, batchNo string) (*model.Inventory, error) {
	var inv model.Inventory
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("warehouse_id = ? AND location_id = ? AND sku_id = ? AND batch_no = ?",
			warehouseID, locationID, skuID, batchNo).First(&inv).Error
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (r *Repository) Create(tx *gorm.DB, inv *model.Inventory) error {
	return tx.Create(inv).Error
}

// IncreaseQty 累加库存（行已锁定，直接更新）。
func (r *Repository) IncreaseQty(tx *gorm.DB, id int64, stock, available int) error {
	return tx.Model(&model.Inventory{}).Where("id = ?", id).Updates(map[string]any{
		"stock_quantity":     gorm.Expr("stock_quantity + ?", stock),
		"available_quantity": gorm.Expr("available_quantity + ?", available),
		"version":            gorm.Expr("version + 1"),
	}).Error
}

// AllocateQty 防超卖第二层：WHERE 条件防护，affected rows = 0 判定并发冲突。
// UPDATE ... SET available = available - ?, allocated = allocated + ? WHERE id = ? AND available >= ?
func (r *Repository) AllocateQty(tx *gorm.DB, id int64, qty int) (int64, error) {
	res := tx.Model(&model.Inventory{}).Where("id = ? AND available_quantity >= ? AND ? > 0", id, qty, qty).
		Updates(map[string]any{
			"available_quantity": gorm.Expr("available_quantity - ?", qty),
			"allocated_quantity": gorm.Expr("allocated_quantity + ?", qty),
			"version":            gorm.Expr("version + 1"),
		})
	return res.RowsAffected, res.Error
}

// ShipQty 发货扣减：WHERE stock >= ? AND allocated >= ? 双条件防负。
func (r *Repository) ShipQty(tx *gorm.DB, id int64, qty int) (int64, error) {
	res := tx.Model(&model.Inventory{}).Where("id = ? AND stock_quantity >= ? AND allocated_quantity >= ? AND ? > 0", id, qty, qty, qty).
		Updates(map[string]any{
			"stock_quantity":     gorm.Expr("stock_quantity - ?", qty),
			"allocated_quantity": gorm.Expr("allocated_quantity - ?", qty),
			"version":            gorm.Expr("version + 1"),
		})
	return res.RowsAffected, res.Error
}

// ReleaseQty 取消分配：WHERE allocated >= ?。
func (r *Repository) ReleaseQty(tx *gorm.DB, id int64, qty int) (int64, error) {
	res := tx.Model(&model.Inventory{}).Where("id = ? AND allocated_quantity >= ? AND ? > 0", id, qty, qty).
		Updates(map[string]any{
			"available_quantity": gorm.Expr("available_quantity + ?", qty),
			"allocated_quantity": gorm.Expr("allocated_quantity - ?", qty),
			"version":            gorm.Expr("version + 1"),
		})
	return res.RowsAffected, res.Error
}

// AdjustNegative 盘点调减：调减量不允许吃掉已分配库存，要求 available >= 减量。
func (r *Repository) AdjustNegative(tx *gorm.DB, id int64, qty int) (int64, error) {
	res := tx.Model(&model.Inventory{}).Where("id = ? AND available_quantity >= ? AND ? > 0", id, qty, qty).
		Updates(map[string]any{
			"stock_quantity":     gorm.Expr("stock_quantity - ?", qty),
			"available_quantity": gorm.Expr("available_quantity - ?", qty),
			"version":            gorm.Expr("version + 1"),
		})
	return res.RowsAffected, res.Error
}

// AdjustPositive 盘点调增（行已锁定）。
func (r *Repository) AdjustPositive(tx *gorm.DB, id int64, qty int) error {
	return r.IncreaseQty(tx, id, qty, qty)
}

// InsertTrans 同事务写流水（只增不改）。
func (r *Repository) InsertTrans(tx *gorm.DB, trans *model.InventoryTrans) error {
	return tx.Create(trans).Error
}

// ---------- 查询（Handler 用，非事务） ----------

type QueryFilter struct {
	WarehouseID int64
	LocationID  int64
	SKUID       int64
	SKUKeyword  string
	// InStockOnly 为 true 时只返回现存量大于 0 的库存行（隐藏已清空的批次）。
	InStockOnly bool
	Page, Size  int
}

func (r *Repository) List(ctx context.Context, db *gorm.DB, f *QueryFilter) ([]*model.Inventory, int64, error) {
	q := db.WithContext(ctx).Model(&model.Inventory{})
	if f.WarehouseID > 0 {
		q = q.Where("warehouse_id = ?", f.WarehouseID)
	}
	if f.LocationID > 0 {
		q = q.Where("location_id = ?", f.LocationID)
	}
	if f.SKUID > 0 {
		q = q.Where("sku_id = ?", f.SKUID)
	}
	if f.SKUKeyword != "" {
		keyword := "%" + dbutil.LikePattern(f.SKUKeyword) + "%"
		if tenantID, scoped := tenant.Scope(ctx); scoped {
			q = q.Where("sku_id IN (SELECT id FROM wms_sku WHERE tenant_id = ? AND deleted_at IS NULL AND (code LIKE ? OR name LIKE ? OR barcode LIKE ?))",
				tenantID, keyword, keyword, keyword)
		} else {
			q = q.Where("sku_id IN (SELECT id FROM wms_sku WHERE deleted_at IS NULL AND (code LIKE ? OR name LIKE ? OR barcode LIKE ?))",
				keyword, keyword, keyword)
		}
	}
	if f.InStockOnly {
		q = q.Where("stock_quantity > 0")
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.Inventory
	err := q.Order("id DESC").Offset((f.Page - 1) * f.Size).Limit(f.Size).Find(&list).Error
	return list, total, err
}

// SummaryBySKU 按 SKU 汇总视图。
// Table 别名联查无 Schema（全局租户回调不注入），此处手动按 ctx 租户过滤。
func (r *Repository) SummaryBySKU(ctx context.Context, db *gorm.DB, warehouseID int64, page, size int) ([]map[string]any, int64, error) {
	q := db.WithContext(ctx).Table("wms_inventory i").
		Joins("JOIN wms_sku s ON s.id = i.sku_id AND s.deleted_at IS NULL AND s.tenant_id = i.tenant_id").
		Where("i.deleted_at IS NULL")
	if tid, scoped := tenant.Scope(ctx); scoped {
		q = q.Where("i.tenant_id = ?", tid)
	}
	if warehouseID > 0 {
		q = q.Where("i.warehouse_id = ?", warehouseID)
	}
	var total int64
	if err := q.Session(&gorm.Session{}).Distinct("i.sku_id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []map[string]any
	err := q.Select("i.sku_id, s.code AS sku_code, s.name AS sku_name, s.unit, " +
		"SUM(i.stock_quantity) AS stock_quantity, SUM(i.available_quantity) AS available_quantity, " +
		"SUM(i.allocated_quantity) AS allocated_quantity").
		Group("i.sku_id, s.code, s.name, s.unit").
		Order("i.sku_id").
		Offset((page - 1) * size).Limit(size).Scan(&list).Error
	return list, total, err
}

func (r *Repository) ListTrans(ctx context.Context, db *gorm.DB, inventoryID int64, orderNo, transType string, page, size int) ([]*model.InventoryTrans, int64, error) {
	q := db.WithContext(ctx).Model(&model.InventoryTrans{})
	if inventoryID > 0 {
		q = q.Where("inventory_id = ?", inventoryID)
	}
	if orderNo != "" {
		q = q.Where("order_no LIKE ?", "%"+dbutil.LikePattern(orderNo)+"%")
	}
	if transType != "" {
		q = q.Where("trans_type = ?", transType)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.InventoryTrans
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// HasStockByWarehouse / HasStockByLocation / HasStockBySKU 删除校验。
// 只要存在库存行就拒绝删除，即使当前数量为零；库存行本身仍是历史引用。
func (r *Repository) HasStockByWarehouse(ctx context.Context, db *gorm.DB, warehouseID int64) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&model.Inventory{}).Where("warehouse_id = ?", warehouseID).Count(&n).Error
	return n > 0, err
}

func (r *Repository) HasStockByLocation(ctx context.Context, db *gorm.DB, locationID int64) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&model.Inventory{}).Where("location_id = ?", locationID).Count(&n).Error
	return n > 0, err
}

func (r *Repository) HasStockBySKU(ctx context.Context, db *gorm.DB, skuID int64) (bool, error) {
	var n int64
	err := db.WithContext(ctx).Model(&model.Inventory{}).Where("sku_id = ?", skuID).Count(&n).Error
	return n > 0, err
}
