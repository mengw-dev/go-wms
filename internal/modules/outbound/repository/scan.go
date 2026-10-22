package repository

import (
	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/tenant"
)

// ScanOrderSKU resolves exact codes only among this order's live tenant-owned lines.
func (r *Repository) ScanOrderSKU(db *gorm.DB, orderID int64, code string) ([]*dto.ScanSKU, error) {
	rows := []*dto.ScanSKU{}
	err := db.Table("wms_shipment_order_detail d").
		Joins("JOIN wms_shipment_order o ON o.id=d.order_id AND o.tenant_id=d.tenant_id AND o.deleted_at IS NULL").
		Joins("JOIN wms_sku s ON s.id=d.sku_id AND s.tenant_id=d.tenant_id AND s.deleted_at IS NULL AND s.status=1").
		Where("d.order_id=? AND d.tenant_id=? AND d.deleted_at IS NULL", orderID, tenant.FromContext(db.Statement.Context)).
		Where("BINARY s.barcode=? OR BINARY s.code=?", code, code).
		Select("DISTINCT s.id,s.code,s.name,s.barcode").Order("s.id").Limit(2).Scan(&rows).Error
	return rows, err
}
