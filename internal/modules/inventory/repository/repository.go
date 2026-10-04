package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
)

type Repository struct{}

// SummaryRow 是库存按 SKU 汇总的数据库投影。
type SummaryRow struct {
	SKUID         int64  `gorm:"column:sku_id"`
	SKUCode       string `gorm:"column:sku_code"`
	SKUName       string `gorm:"column:sku_name"`
	Unit          string `gorm:"column:unit"`
	StockQuantity int64  `gorm:"column:stock_quantity"`
	AvailableQty  int64  `gorm:"column:available_quantity"`
	AllocatedQty  int64  `gorm:"column:allocated_quantity"`
}

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
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sku, skuID).Error
}

// FIFOCandidate 是候选库存行的轻量投影，只带排序和用量估算需要的列。
type FIFOCandidate struct {
	ID           int64     `gorm:"column:id"`
	AvailableQty int       `gorm:"column:available_quantity"`
	StockInTime  time.Time `gorm:"column:stock_in_time"`
}

// ListFIFOCandidates 按 FIFO 顺序取一批候选库存行，**不加行锁**。
//
// 这是缩小锁范围的第一步：昂贵的 FIFO 排序放在普通读里完成，只花 CPU 不产生行锁；
// 加锁交给 LockInventoryByIDs，锁集合收敛到调用方选中的候选。
//
// afterStockInTime / afterID 是上一批的最后一个键，用于向后翻页，首批传零值。
// 候选基于当前事务的一致性快照，可能已经过期（并发下更明显），
// 因此调用方必须在拿到行锁后重新读取可用量再决定扣减。
//
// 边界：普通读用的是事务快照，看不见本次事务开始后才入库的新行。
// 极端情况下（审核事务开始后有人上架补货）可能报一次"可用不足"，
// 下一次请求就能看到新库存；旧实现用 FOR UPDATE 读最新已提交数据，没有这个差异。
func (r *Repository) ListFIFOCandidates(tx *gorm.DB, warehouseID, skuID int64, limit int, afterStockInTime time.Time, afterID int64) ([]FIFOCandidate, error) {
	var list []FIFOCandidate
	q := tx.Table("wms_inventory i").
		Select("i.id, i.available_quantity, i.stock_in_time").
		Where("i.warehouse_id = ? AND i.sku_id = ? AND i.available_quantity > 0 AND i.deleted_at IS NULL", warehouseID, skuID)
	if tenantID, scoped := tenant.Scope(tx.Statement.Context); scoped {
		q = q.Where("i.tenant_id = ?", tenantID)
	}
	if afterID > 0 {
		q = q.Where("(i.stock_in_time, i.id) > (?, ?)", afterStockInTime, afterID)
	}
	err := q.Order("i.stock_in_time ASC, i.id ASC").Limit(limit).Scan(&list).Error
	return list, err
}

// LockInventoryByIDs 只锁指定主键的库存行（防超卖第一层：行锁串行化同一库存行的并发分配）。
//
// 不写 ORDER BY：InnoDB 按主键升序逐行加锁，所有事务加锁顺序一致，不会形成加锁顺序环；
// FIFO 的取用顺序由调用方在锁内自行排序决定。
func (r *Repository) LockInventoryByIDs(tx *gorm.DB, ids []int64) ([]*model.Inventory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var list []*model.Inventory
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id IN ?", ids).
		Find(&list).Error
	return list, err
}

// ListLocationCodes 查询库位编码，供分配结果回填。普通读，不参与加锁。
// 锁读里不再 JOIN wms_location，避免把库位行一起锁住。
func (r *Repository) ListLocationCodes(tx *gorm.DB, ids []int64) (map[int64]string, error) {
	codes := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return codes, nil
	}
	var rows []struct {
		ID   int64  `gorm:"column:id"`
		Code string `gorm:"column:code"`
	}
	q := tx.Table("wms_location l").Select("l.id, l.code").Where("l.id IN ? AND l.deleted_at IS NULL", ids)
	if tenantID, scoped := tenant.Scope(tx.Statement.Context); scoped {
		q = q.Where("l.tenant_id = ?", tenantID)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		codes[row.ID] = row.Code
	}
	return codes, nil
}

// SumAvailableQty 汇总某仓库/SKU 的可用量，只在分配失败时用于生成准确的错误提示。
// 与候选查询同用事务快照：调用方在未观察到快照过期时才走这里，此时快照与真实值一致。
func (r *Repository) SumAvailableQty(tx *gorm.DB, warehouseID, skuID int64) (int, error) {
	var row struct {
		Total int `gorm:"column:total"`
	}
	q := tx.Table("wms_inventory i").
		Select("COALESCE(SUM(i.available_quantity), 0) AS total").
		Where("i.warehouse_id = ? AND i.sku_id = ? AND i.deleted_at IS NULL", warehouseID, skuID)
	if tenantID, scoped := tenant.Scope(tx.Statement.Context); scoped {
		q = q.Where("i.tenant_id = ?", tenantID)
	}
	if err := q.Scan(&row).Error; err != nil {
		return 0, err
	}
	return row.Total, nil
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
func (r *Repository) SummaryBySKU(ctx context.Context, db *gorm.DB, warehouseID int64, page, size int) ([]*SummaryRow, int64, error) {
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
	var list []*SummaryRow
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
