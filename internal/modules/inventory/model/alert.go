package model

import "gowms/internal/pkg/modelbase"

// AlertRule configures a warehouse/SKU's available quantity floor. It does not
// own stock or persist an alert occurrence; current alerts are derived on read.
type AlertRule struct {
	modelbase.Base
	TenantID        int64  `gorm:"not null;default:0;uniqueIndex:uk_alert_rule,priority:1"`
	WarehouseID     int64  `gorm:"not null;uniqueIndex:uk_alert_rule,priority:2"`
	SKUID           int64  `gorm:"column:sku_id;not null;uniqueIndex:uk_alert_rule,priority:3"`
	MinAvailableQty int    `gorm:"type:int;size:32;not null;check:chk_alert_min_qty,min_available_qty > 0"`
	Enabled         bool   `gorm:"not null"`
	UpdatedBy       string `gorm:"size:64;not null"`
}

func (AlertRule) TableName() string { return "wms_inventory_alert_rule" }
