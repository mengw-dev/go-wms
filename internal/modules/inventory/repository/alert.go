package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/errcode"
)

// AlertReferences shares catalog locks with deletion/disable operations. Rules
// never lock or modify inventory rows. Caller owns the transaction and tenant.
func (r *Repository) AlertReferences(db *gorm.DB, warehouseID, skuID int64) error {
	var warehouse basicmodel.Warehouse
	if err := db.Clauses(clause.Locking{Strength: "SHARE"}).First(&warehouse, warehouseID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.WarehouseNotFound
		}
		return err
	}
	if warehouse.Status != 1 {
		return errcode.WarehouseDisabled
	}
	var sku basicmodel.SKU
	if err := db.Clauses(clause.Locking{Strength: "SHARE"}).First(&sku, skuID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.SKUNotFound
		}
		return err
	}
	if sku.Status != 1 {
		return errcode.SKUDisabled
	}
	return nil
}

func (r *Repository) SaveAlertRule(db *gorm.DB, rule *model.AlertRule) (int64, error) {
	err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "tenant_id"}, {Name: "warehouse_id"}, {Name: "sku_id"}}, DoUpdates: clause.Assignments(map[string]any{"min_available_qty": rule.MinAvailableQty, "enabled": rule.Enabled, "updated_by": rule.UpdatedBy, "updated_at": time.Now()})}).Create(rule).Error
	if err != nil {
		return 0, err
	}
	var saved model.AlertRule
	err = db.Where("warehouse_id = ? AND sku_id = ?", rule.WarehouseID, rule.SKUID).First(&saved).Error
	return saved.ID, err
}

func (r *Repository) DeleteAlertRule(db *gorm.DB, id int64) error {
	// Rules are configuration, not stock/history. Removal frees its unique tuple.
	return db.Unscoped().Delete(&model.AlertRule{}, id).Error
}

func (r *Repository) AlertRows(db *gorm.DB, tenantID int64, q *dto.AlertQuery) *gorm.DB {
	stock := db.Table("wms_inventory").Select("warehouse_id,sku_id,SUM(stock_quantity) AS stock,SUM(available_quantity) AS available,SUM(allocated_quantity) AS allocated").Where("tenant_id = ? AND deleted_at IS NULL", tenantID)
	if q.WarehouseID > 0 {
		stock = stock.Where("warehouse_id = ?", q.WarehouseID)
	}
	stock = stock.Group("warehouse_id,sku_id")
	// Every raw table and join has an explicit tenant and soft-delete boundary,
	// including exact platform tenant 0. LEFT JOIN keeps zero-row stock visible.
	base := db.Table("wms_inventory_alert_rule r").
		Joins("LEFT JOIN wms_warehouse w ON w.id=r.warehouse_id AND w.tenant_id=r.tenant_id AND w.deleted_at IS NULL").
		Joins("LEFT JOIN wms_sku s ON s.id=r.sku_id AND s.tenant_id=r.tenant_id AND s.deleted_at IS NULL").
		Joins("LEFT JOIN (?) v ON v.warehouse_id=r.warehouse_id AND v.sku_id=r.sku_id", stock).
		Where("r.tenant_id = ? AND r.deleted_at IS NULL", tenantID)
	if q.WarehouseID > 0 {
		base = base.Where("r.warehouse_id = ?", q.WarehouseID)
	}
	if q.SKUKeyword != "" {
		word := "%" + dbutil.LikePattern(q.SKUKeyword) + "%"
		base = base.Where("s.code LIKE ? OR s.name LIKE ? OR s.barcode LIKE ?", word, word, word)
	}
	return base.Select(`r.id,r.warehouse_id,r.sku_id,r.min_available_qty,r.enabled,r.updated_by,r.updated_at,
COALESCE(w.code,'') AS warehouse_code,COALESCE(w.name,'') AS warehouse_name,
COALESCE(s.code,'') AS sku_code,COALESCE(s.name,'') AS sku_name,COALESCE(s.unit,'') AS unit,
COALESCE(v.stock,0) AS stock_quantity,COALESCE(v.available,0) AS available_qty,COALESCE(v.allocated,0) AS allocated_qty,
GREATEST(r.min_available_qty-COALESCE(v.available,0),0) AS gap_qty,
CASE WHEN r.enabled=0 THEN 'PAUSED'
WHEN w.id IS NULL OR s.id IS NULL OR w.status<>1 OR s.status<>1 THEN 'UNAVAILABLE'
WHEN COALESCE(v.available,0)=0 THEN 'OUT_OF_STOCK'
WHEN v.available<r.min_available_qty THEN 'LOW' ELSE 'NORMAL' END AS state`)
}

func (r *Repository) ListAlerts(db *gorm.DB, tenantID int64, q *dto.AlertQuery) (*dto.AlertListResp, error) {
	result := &dto.AlertListResp{List: make([]*dto.AlertRow, 0), CheckedAt: time.Now()}
	result.Warehouses = make([]*dto.AlertOption, 0)
	if err := db.Table("wms_inventory_alert_rule r").Joins("JOIN wms_warehouse w ON w.id=r.warehouse_id AND w.tenant_id=r.tenant_id AND w.deleted_at IS NULL").Where("r.tenant_id=? AND r.deleted_at IS NULL", tenantID).Select("DISTINCT w.id,w.code,w.name").Order("w.code,w.id").Scan(&result.Warehouses).Error; err != nil {
		return nil, err
	}
	base := r.AlertRows(db, tenantID, q)
	if err := db.Table("(?) a", base).Select("COUNT(*) AS configured,COALESCE(SUM(state='OUT_OF_STOCK'),0) AS out_of_stock,COALESCE(SUM(state='LOW'),0) AS low").Scan(&result.Summary).Error; err != nil {
		return nil, err
	}
	rows := db.Table("(?) a", base)
	if q.State == "ALERT" {
		rows = rows.Where("state IN ?", []string{"OUT_OF_STOCK", "LOW"})
	} else if q.State != "ALL" {
		rows = rows.Where("state = ?", q.State)
	}
	if err := rows.Count(&result.Total).Error; err != nil {
		return nil, err
	}
	err := rows.Order("CASE state WHEN 'OUT_OF_STOCK' THEN 0 WHEN 'LOW' THEN 1 ELSE 2 END,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&result.List).Error
	return result, err
}

func (r *Repository) AlertOptions(db *gorm.DB, q *dto.AlertOptionQuery) ([]*dto.AlertOption, int64, error) {
	var rows = make([]*dto.AlertOption, 0)
	var total int64
	var query *gorm.DB
	if q.Kind == "warehouse" {
		query = db.Model(&basicmodel.Warehouse{})
	} else {
		query = db.Model(&basicmodel.SKU{})
	}
	query = query.Where("status=1")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	fields := "id,code,name,unit"
	if q.Kind == "warehouse" {
		fields = "id,code,name"
	}
	err := query.Select(fields).Order("code,id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&rows).Error
	return rows, total, err
}
