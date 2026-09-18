package repository

import (
	"context"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/tenant"
)

// 只读检索层：为 AI 问答提供真实库存快照（检索增强）。
// 所有查询均走 GORM（软删除自动过滤主模型表；联查表手动补 deleted_at 条件）。

// tenantScope 别名联查手动注入租户条件：Table("... i") 的裸表查询无 Schema，
// 全局 GORM 回调不会注入 tenant_id，必须在此显式过滤（AI 快照绝不能跨租户泄漏）。
func tenantScope(ctx context.Context, q *gorm.DB, alias string) *gorm.DB {
	if tid := tenant.FromContext(ctx); tid > 0 {
		return q.Where(alias+".tenant_id = ?", tid)
	}
	return q
}

// Overview 全局库存概览。
type Overview struct {
	SKUTotal       int64 `json:"sku_total"`
	InventoryRows  int64 `json:"inventory_rows"` // 库存记录数（仓库+库位+SKU+批次组合数）
	StockTotal     int64 `json:"stock_total"`    // 库存总量
	AvailableTotal int64 `json:"available_total"`
	AllocatedTotal int64 `json:"allocated_total"`
}

// WarehouseStock 仓库维度库存汇总行。
type WarehouseStock struct {
	WarehouseCode string `json:"warehouse_code"`
	WarehouseName string `json:"warehouse_name"`
	StockQty      int64  `json:"stock_qty"`
	AvailableQty  int64  `json:"available_qty"`
}

// SKUStock SKU 维度库存汇总行。
type SKUStock struct {
	SKUCode      string `json:"sku_code"`
	SKUName      string `json:"sku_name"`
	Spec         string `json:"spec"`
	Unit         string `json:"unit"`
	StockQty     int64  `json:"stock_qty"`
	AvailableQty int64  `json:"available_qty"`
}

// InventoryDetail 库位+批次粒度的库存明细。
type InventoryDetail struct {
	WarehouseCode string `json:"warehouse_code"`
	LocationCode  string `json:"location_code"`
	BatchNo       string `json:"batch_no"`
	StockQty      int    `json:"stock_qty"`
	AvailableQty  int    `json:"available_qty"`
	AllocatedQty  int    `json:"allocated_qty"`
}

// SKUBrief SKU 目录简要字段（用于问题匹配与目录展示）。
type SKUBrief struct {
	ID      int64  `json:"id"`
	Code    string `json:"code"`
	Barcode string `json:"barcode"`
	Name    string `json:"name"`
	Spec    string `json:"spec"`
	Unit    string `json:"unit"`
	Status  int    `json:"status"`
}

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

// GetOverview SKU 总数 + 三数量汇总。
// 注意：GORM 的 Scan(struct) 会先清零目标结构体，因此聚合结果必须扫描到
// 独立结构再拷回，否则先执行的 Count 结果会被清掉。
func (r *Repository) GetOverview(ctx context.Context) (*Overview, error) {
	o := &Overview{}
	if err := r.db.WithContext(ctx).Model(&basicmodel.SKU{}).Count(&o.SKUTotal).Error; err != nil {
		return nil, err
	}
	var agg Overview
	err := r.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Select("COUNT(*) AS inventory_rows, "+
			"COALESCE(SUM(stock_quantity),0) AS stock_total, "+
			"COALESCE(SUM(available_quantity),0) AS available_total, "+
			"COALESCE(SUM(allocated_quantity),0) AS allocated_total").
		Scan(&agg).Error
	if err != nil {
		return nil, err
	}
	o.InventoryRows = agg.InventoryRows
	o.StockTotal = agg.StockTotal
	o.AvailableTotal = agg.AvailableTotal
	o.AllocatedTotal = agg.AllocatedTotal
	return o, nil
}

// ListWarehouseStock 各仓库库存分布。
func (r *Repository) ListWarehouseStock(ctx context.Context) ([]WarehouseStock, error) {
	var list []WarehouseStock
	q := tenantScope(ctx, r.db.WithContext(ctx).Table("wms_inventory i").
		Select("w.code AS warehouse_code, w.name AS warehouse_name, "+
			"SUM(i.stock_quantity) AS stock_qty, SUM(i.available_quantity) AS available_qty").
		Joins("JOIN wms_warehouse w ON w.id = i.warehouse_id AND w.deleted_at IS NULL AND w.tenant_id = i.tenant_id").
		Where("i.deleted_at IS NULL"), "i")
	err := q.Group("w.id, w.code, w.name").
		Order("stock_qty DESC").
		Scan(&list).Error
	return list, err
}

// TopSKUByStock 库存总量最多的前 N 个 SKU。
func (r *Repository) TopSKUByStock(ctx context.Context, limit int) ([]SKUStock, error) {
	var list []SKUStock
	q := tenantScope(ctx, r.db.WithContext(ctx).Table("wms_inventory i").
		Select("s.code AS sku_code, s.name AS sku_name, s.spec, s.unit, "+
			"SUM(i.stock_quantity) AS stock_qty, SUM(i.available_quantity) AS available_qty").
		Joins("JOIN wms_sku s ON s.id = i.sku_id AND s.deleted_at IS NULL AND s.tenant_id = i.tenant_id").
		Where("i.deleted_at IS NULL"), "i")
	err := q.Group("s.id, s.code, s.name, s.spec, s.unit").
		Order("stock_qty DESC").
		Limit(limit).
		Scan(&list).Error
	return list, err
}

// LowAvailableSKU 可用库存偏低的 SKU（升序，含可用为 0）。
func (r *Repository) LowAvailableSKU(ctx context.Context, threshold, limit int) ([]SKUStock, error) {
	var list []SKUStock
	q := tenantScope(ctx, r.db.WithContext(ctx).Table("wms_inventory i").
		Select("s.code AS sku_code, s.name AS sku_name, s.spec, s.unit, "+
			"SUM(i.stock_quantity) AS stock_qty, SUM(i.available_quantity) AS available_qty").
		Joins("JOIN wms_sku s ON s.id = i.sku_id AND s.deleted_at IS NULL AND s.tenant_id = i.tenant_id").
		Where("i.deleted_at IS NULL AND i.available_quantity <= ?", threshold), "i")
	err := q.Group("s.id, s.code, s.name, s.spec, s.unit").
		Order("available_qty ASC").
		Limit(limit).
		Scan(&list).Error
	return list, err
}

// ListSKUBrief SKU 目录（按编码排序，限量防止提示词过长）。
func (r *Repository) ListSKUBrief(ctx context.Context, limit int) ([]SKUBrief, error) {
	var list []SKUBrief
	err := r.db.WithContext(ctx).Model(&basicmodel.SKU{}).
		Select("id, code, barcode, name, spec, unit, status").
		Order("code ASC").
		Limit(limit).
		Scan(&list).Error
	return list, err
}

// ListInventoryDetailBySKU 指定 SKU 的库存明细（库位/批次粒度，可用量降序）。
func (r *Repository) ListInventoryDetailBySKU(ctx context.Context, skuID int64, limit int) ([]InventoryDetail, error) {
	var list []InventoryDetail
	q := tenantScope(ctx, r.db.WithContext(ctx).Table("wms_inventory i").
		Select("w.code AS warehouse_code, l.code AS location_code, i.batch_no, "+
			"i.stock_quantity AS stock_qty, i.available_quantity AS available_qty, i.allocated_quantity AS allocated_qty").
		Joins("JOIN wms_warehouse w ON w.id = i.warehouse_id AND w.deleted_at IS NULL AND w.tenant_id = i.tenant_id").
		Joins("JOIN wms_location l ON l.id = i.location_id AND l.deleted_at IS NULL AND l.tenant_id = i.tenant_id").
		Where("i.deleted_at IS NULL AND i.sku_id = ?", skuID), "i")
	err := q.Order("i.available_quantity DESC, i.id ASC").
		Limit(limit).
		Scan(&list).Error
	return list, err
}
