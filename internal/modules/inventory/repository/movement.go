package repository

import (
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

func (r *Repository) MoveSource(db *gorm.DB, id int64) (*dto.MoveSource, error) {
	var out dto.MoveSource
	err := db.Table("wms_inventory i").
		Joins("JOIN wms_warehouse w ON w.id=i.warehouse_id AND w.tenant_id=i.tenant_id AND w.deleted_at IS NULL").
		Joins("JOIN wms_location l ON l.id=i.location_id AND l.tenant_id=i.tenant_id AND l.warehouse_id=i.warehouse_id AND l.deleted_at IS NULL").
		Joins("JOIN wms_sku s ON s.id=i.sku_id AND s.tenant_id=i.tenant_id AND s.deleted_at IS NULL").
		Where("i.id=? AND i.tenant_id=? AND i.deleted_at IS NULL", id, tenant.FromContext(db.Statement.Context)).
		Select("i.id,i.warehouse_id,w.code AS warehouse_code,l.code AS location_code,s.code AS sku_code,s.name AS sku_name,s.unit,i.batch_no,i.available_quantity AS available_qty,i.stock_in_time,(w.status=1 AND l.status<>0 AND s.status=1) AS enabled").Scan(&out).Error
	if err == nil && out.ID == 0 {
		return nil, errcode.InventoryNotFound
	}
	return &out, err
}

func (r *Repository) MovementSource(db *gorm.DB, id int64) (*model.Inventory, error) {
	var row model.Inventory
	err := db.First(&row, id).Error
	return &row, err
}
func (r *Repository) SaveMovement(db *gorm.DB, row *model.Movement) error {
	err := db.Create(row).Error
	if pkgtx.IsDuplicateErr(err) {
		return errcode.Conflict
	}
	return err
}

func (r *Repository) FindMovement(db *gorm.DB, key string) (*model.Movement, error) {
	var m model.Movement
	err := db.Where("operation_key = ?", key).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &m, err
}

// LockMoveReferences shares warehouse/SKU references and exclusively locks ordered
// locations before inventory, preserving deletion/disable protection without a warehouse gate.
func (r *Repository) LockMoveReferences(db *gorm.DB, source *model.Inventory, target int64) (*basicmodel.Warehouse, []*basicmodel.Location, *basicmodel.SKU, error) {
	var w basicmodel.Warehouse
	if err := db.Clauses(clause.Locking{Strength: "SHARE"}).First(&w, source.WarehouseID).Error; err != nil {
		return nil, nil, nil, err
	}
	ids := []int64{source.LocationID, target}
	slices.Sort(ids)
	var locations []*basicmodel.Location
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", ids).Order("id").Find(&locations).Error; err != nil {
		return nil, nil, nil, err
	}
	if w.Status != 1 || len(locations) != 2 {
		return nil, nil, nil, errcode.MoveNotAllowed
	}
	for _, l := range locations {
		if l.WarehouseID != w.ID || l.Status == 0 {
			return nil, nil, nil, errcode.MoveNotAllowed
		}
	}
	var sku basicmodel.SKU
	if err := db.Clauses(clause.Locking{Strength: "SHARE"}).First(&sku, source.SKUID).Error; err != nil {
		return nil, nil, nil, err
	}
	if sku.Status != 1 {
		return nil, nil, nil, errcode.SKUDisabled
	}
	return &w, locations, &sku, nil
}

func (r *Repository) SetStockInTime(db *gorm.DB, id int64, at time.Time) error {
	return db.Model(&model.Inventory{}).Where("id = ?", id).Update("stock_in_time", at).Error
}

func (r *Repository) SyncMoveLocations(db *gorm.DB, source, target int64) error {
	// A latest locking read, not an earlier repeatable-read snapshot. Incoming
	// stock follows the same warehouse/location gate; existing stock stays locked.
	var positive []model.Inventory
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("location_id=? AND stock_quantity>0", source).Order("id").Limit(1).Find(&positive).Error; err != nil {
		return err
	}
	state := basicmodel.LocationStatusIdle
	if len(positive) > 0 {
		state = basicmodel.LocationStatusOccupied
	}
	if err := db.Model(&basicmodel.Location{}).Where("id=?", source).Update("status", state).Error; err != nil {
		return err
	}
	return db.Model(&basicmodel.Location{}).Where("id=?", target).Update("status", basicmodel.LocationStatusOccupied).Error
}

func (r *Repository) ListMovements(db *gorm.DB, q *dto.MoveQuery) ([]model.Movement, int64, error) {
	query := func() *gorm.DB {
		d := db.Model(&model.Movement{})
		if q.WarehouseID > 0 {
			d = d.Where("warehouse_id=?", q.WarehouseID)
		}
		if q.OperationKey != "" {
			d = d.Where("operation_key=?", q.OperationKey)
		}
		if q.Keyword != "" {
			word := "%" + dbutil.LikePattern(q.Keyword) + "%"
			d = d.Where("move_no LIKE ? OR sku_code LIKE ? OR sku_name LIKE ? OR batch_no LIKE ?", word, word, word, word)
		}
		return d
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	list := []model.Movement{}
	err := query().Order("created_at DESC,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&list).Error
	return list, total, err
}

func (r *Repository) MoveLocations(db *gorm.DB, source *model.Inventory, q *dto.MoveLocationQuery) ([]*dto.AlertOption, int64, error) {
	query := func() *gorm.DB {
		d := db.Model(&basicmodel.Location{}).Where("warehouse_id=? AND id<>? AND status IN ?", source.WarehouseID, source.LocationID, []int{1, 2})
		if q.Keyword != "" {
			d = d.Where("code LIKE ?", "%"+dbutil.LikePattern(q.Keyword)+"%")
		}
		return d
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	list := []*dto.AlertOption{}
	err := query().Select("id,code").Order("code,id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&list).Error
	return list, total, err
}
