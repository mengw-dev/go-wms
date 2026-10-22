package repository

import (
	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
)

// PutawayLocations lists enabled locations only in the tenant and order's warehouse.
func (r *Repository) PutawayLocations(db *gorm.DB, id int64, q *dto.PutawayLocationQuery) ([]*dto.PutawayLocation, int64, error) {
	query := func() *gorm.DB {
		d := db.Table("wms_location l").Joins("JOIN wms_warehouse w ON w.id=l.warehouse_id AND w.tenant_id=l.tenant_id AND w.deleted_at IS NULL AND w.status=1").
			Joins("JOIN wms_receipt_order o ON o.warehouse_id=w.id AND o.tenant_id=w.tenant_id AND o.deleted_at IS NULL").
			Where("o.id=? AND l.tenant_id=? AND l.deleted_at IS NULL AND l.status IN (1,2)", id, tenant.FromContext(db.Statement.Context))
		if q.Code != "" {
			d = d.Where("BINARY l.code=?", q.Code)
		}
		if q.Keyword != "" {
			d = d.Where("l.code LIKE ?", "%"+dbutil.LikePattern(q.Keyword)+"%")
		}
		return d
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := []*dto.PutawayLocation{}
	err := query().Select("l.id,l.code,l.status").Order("l.code,l.id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&rows).Error
	return rows, total, err
}
