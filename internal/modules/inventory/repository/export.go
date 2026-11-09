package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
)

type ExportStockRow struct {
	ID                int64
	WarehouseID       int64
	WarehouseCode     string
	WarehouseName     string
	LocationID        int64
	LocationCode      string
	SKUID             int64 `gorm:"column:sku_id"`
	SKUCode           string
	SKUName           string
	Barcode           string
	Unit              string
	BatchNo           string
	StockQuantity     int
	AvailableQuantity int
	FrozenQuantity    int
	AllocatedQuantity int
	StockInTime       time.Time
}

// ExportStock uses one bounded SELECT. LEFT JOIN keeps orphaned historical rows;
// every catalog join checks tenant and soft deletion independently.
func (r *Repository) ExportStock(ctx context.Context, db *gorm.DB, f *dto.InventoryExportQuery, limit int) ([]ExportStockRow, error) {
	tid := tenant.FromContext(ctx)
	q := db.WithContext(ctx).Table("wms_inventory i").
		Joins("LEFT JOIN wms_warehouse w ON w.id=i.warehouse_id AND w.tenant_id=i.tenant_id AND w.deleted_at IS NULL").
		Joins("LEFT JOIN wms_location l ON l.id=i.location_id AND l.tenant_id=i.tenant_id AND l.deleted_at IS NULL").
		Joins("LEFT JOIN wms_sku s ON s.id=i.sku_id AND s.tenant_id=i.tenant_id AND s.deleted_at IS NULL").
		Where("i.tenant_id=? AND i.deleted_at IS NULL", tid)
	if f.WarehouseID > 0 {
		q = q.Where("i.warehouse_id=?", f.WarehouseID)
	}
	if f.LocationID > 0 {
		q = q.Where("i.location_id=?", f.LocationID)
	}
	if f.SKUID > 0 {
		q = q.Where("i.sku_id=?", f.SKUID)
	}
	if f.SKUKeyword != "" {
		key := "%" + dbutil.LikePattern(f.SKUKeyword) + "%"
		q = q.Where("(s.code LIKE ? OR s.name LIKE ? OR s.barcode LIKE ?)", key, key, key)
	}
	if f.InStockOnly {
		q = q.Where("i.stock_quantity>0")
	}
	var rows []ExportStockRow
	err := q.Select("i.id,i.warehouse_id,w.code AS warehouse_code,w.name AS warehouse_name,i.location_id,l.code AS location_code,i.sku_id,s.code AS sku_code,s.name AS sku_name,s.barcode,s.unit,i.batch_no,i.stock_quantity,i.available_quantity,i.allocated_quantity,i.frozen_quantity,i.stock_in_time").Order("i.id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

type ExportTransRow struct {
	ID              int64
	InventoryID     int64
	WarehouseID     int64
	WarehouseCode   string
	LocationCode    string
	SKUCode         string
	SKUName         string
	Unit            string
	BatchNo         string
	TransType       string
	QuantityChange  int
	BeforeQuantity  int
	AfterQuantity   int
	AvailableBefore int
	FrozenBefore    int
	FrozenAfter     int
	Reason          string
	AvailableAfter  int
	OrderNo         string
	TaskNo          string
	Operator        string
	CreatedAt       time.Time
}

// ExportTransactions preserves immutable ledger rows even when catalog/stock is deleted.
func (r *Repository) ExportTransactions(ctx context.Context, db *gorm.DB, f *dto.TransExportQuery, limit int) ([]ExportTransRow, error) {
	q := db.WithContext(ctx).Table("wms_inventory_trans t").
		Joins("LEFT JOIN wms_inventory i ON i.id=t.inventory_id AND i.tenant_id=t.tenant_id AND i.deleted_at IS NULL").
		Joins("LEFT JOIN wms_warehouse w ON w.id=i.warehouse_id AND w.tenant_id=t.tenant_id AND w.deleted_at IS NULL").
		Joins("LEFT JOIN wms_location l ON l.id=i.location_id AND l.tenant_id=t.tenant_id AND l.deleted_at IS NULL").
		Joins("LEFT JOIN wms_sku s ON s.id=i.sku_id AND s.tenant_id=t.tenant_id AND s.deleted_at IS NULL").
		Where("t.tenant_id=?", tenant.FromContext(ctx))
	if f.InventoryID > 0 {
		q = q.Where("t.inventory_id=?", f.InventoryID)
	}
	if f.OrderNo != "" {
		q = q.Where("t.order_no LIKE ?", "%"+dbutil.LikePattern(f.OrderNo)+"%")
	}
	if f.TransType != "" {
		q = q.Where("t.trans_type=?", f.TransType)
	}
	var rows []ExportTransRow
	err := q.Select("t.id,t.inventory_id,i.warehouse_id,w.code AS warehouse_code,l.code AS location_code,s.code AS sku_code,s.name AS sku_name,s.unit,i.batch_no,t.trans_type,t.quantity_change,t.before_quantity,t.after_quantity,t.available_before,t.available_after,t.frozen_before,t.frozen_after,t.reason,t.order_no,t.task_no,t.operator,t.created_at").Order("t.id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}
