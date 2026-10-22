package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
)

type ExportRow struct {
	OrderID      int64
	OrderNo      string
	WarehouseID  int64
	Status       string
	DetailID     int64
	InventoryID  int64
	SKUCode      string
	SKUName      string
	LocationCode string
	BatchNo      string
	BookQty      int
	ActualQty    *int
	DiffQty      int
	Adjusted     bool
	CreatedAt    time.Time
}

// ExportDetails keeps snapshot identifiers and separates snapshot variance from approved adjustment.
func (r *Repository) ExportDetails(ctx context.Context, db *gorm.DB, f *dto.ExportQuery, limit int) ([]ExportRow, error) {
	q := db.WithContext(ctx).Table("wms_stocktake_detail d").
		Joins("JOIN wms_stocktake_order o ON o.id=d.order_id AND o.tenant_id=d.tenant_id AND o.deleted_at IS NULL").
		Where("d.tenant_id=? AND d.deleted_at IS NULL", tenant.FromContext(ctx))
	if f.WarehouseID > 0 {
		q = q.Where("o.warehouse_id=?", f.WarehouseID)
	}
	if f.OrderID > 0 {
		q = q.Where("o.id=?", f.OrderID)
	}
	if f.Status != "" {
		q = q.Where("o.status=?", f.Status)
	}
	if f.Keyword != "" {
		q = q.Where("o.order_no LIKE ?", "%"+dbutil.LikePattern(f.Keyword)+"%")
	}
	if f.DifferencesOnly {
		q = q.Where("d.actual_qty IS NOT NULL AND (d.actual_qty<>d.book_qty OR (o.status='COMPLETED' AND d.diff_qty<>0))")
	}
	var rows []ExportRow
	err := q.Select("o.id AS order_id,o.order_no,o.warehouse_id,o.status,d.id AS detail_id,d.inventory_id,d.sku_code,d.sku_name,d.location_code,d.batch_no,d.book_qty,d.actual_qty,d.diff_qty,d.adjusted,o.created_at").Order("o.id DESC,d.id ASC").Limit(limit).Scan(&rows).Error
	return rows, err
}
