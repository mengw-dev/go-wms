package model

import "time"

// Movement is an immutable receipt and durable deduplication boundary. No delete
// endpoint or expiry: a replay must never execute a historical stock move again.
type Movement struct {
	ID                 int64     `gorm:"primaryKey"`
	TenantID           int64     `gorm:"not null;default:0;uniqueIndex:uk_move_operation,priority:1;index:idx_move_tenant_time,priority:1"`
	OperationKey       string    `gorm:"size:64;not null;uniqueIndex:uk_move_operation,priority:2"`
	RequestHash        string    `gorm:"size:64;not null"`
	MoveNo             string    `gorm:"size:64;not null"`
	WarehouseID        int64     `gorm:"not null"`
	WarehouseCode      string    `gorm:"size:32;not null"`
	SourceInventoryID  int64     `gorm:"not null"`
	TargetInventoryID  int64     `gorm:"not null"`
	SourceLocationID   int64     `gorm:"not null"`
	TargetLocationID   int64     `gorm:"not null"`
	SourceLocationCode string    `gorm:"size:64;not null"`
	TargetLocationCode string    `gorm:"size:64;not null"`
	SKUID              int64     `gorm:"column:sku_id;not null"`
	SKUCode            string    `gorm:"size:64;not null"`
	SKUName            string    `gorm:"size:128;not null"`
	Unit               string    `gorm:"size:16;not null"`
	BatchNo            string    `gorm:"size:64;not null"`
	Quantity           int       `gorm:"type:int;size:32;not null;check:chk_move_positive,quantity > 0"`
	StockInTime        time.Time `gorm:"not null"`
	Operator           string    `gorm:"size:64;not null"`
	Reason             string    `gorm:"size:255;not null"`
	CreatedAt          time.Time `gorm:"not null;index:idx_move_tenant_time,priority:2"`
}

func (Movement) TableName() string { return "wms_inventory_movement" }
