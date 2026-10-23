package model

import "time"

// WorkException is an audit record. Resolutions never erase the original report.
type WorkException struct {
	ID          int64      `json:"id,string" gorm:"primaryKey"`
	TenantID    int64      `json:"tenant_id,string" gorm:"not null;default:0;index:idx_exception_tenant_order,priority:1"`
	OrderID     int64      `json:"order_id,string" gorm:"not null;index:idx_exception_tenant_order,priority:2"`
	OrderNo     string     `json:"order_no" gorm:"size:64;not null"`
	TaskID      int64      `json:"task_id,string" gorm:"not null"`
	InventoryID int64      `json:"inventory_id,string" gorm:"not null"`
	Kind        string     `json:"kind" gorm:"size:16;not null"`
	Quantity    int        `json:"quantity" gorm:"type:int;size:32;not null"`
	Reason      string     `json:"reason" gorm:"size:255;not null"`
	Status      string     `json:"status" gorm:"size:16;not null;default:OPEN"`
	Resolution  string     `json:"resolution" gorm:"size:255;not null;default:''"`
	Operator    string     `json:"operator" gorm:"size:64;not null"`
	ResolvedBy  string     `json:"resolved_by" gorm:"size:64;not null;default:''"`
	CreatedAt   time.Time  `json:"created_at"`
	ResolvedAt  *time.Time `json:"resolved_at"`
}

func (WorkException) TableName() string { return "wms_work_exception" }

// CustomerReturn receives one original allocation/batch per receipt. Multiple
// receipts are allowed, but their sum is capped under the original allocation lock.
type CustomerReturn struct {
	SKUCode      string    `json:"sku_code" gorm:"size:64;not null"`
	SKUName      string    `json:"sku_name" gorm:"size:128;not null"`
	Unit         string    `json:"unit" gorm:"size:16;not null"`
	LocationCode string    `json:"location_code" gorm:"size:64;not null"`
	ID           int64     `json:"id,string" gorm:"primaryKey"`
	TenantID     int64     `json:"tenant_id,string" gorm:"not null;default:0;index:idx_return_tenant_order,priority:1"`
	ReturnNo     string    `json:"return_no" gorm:"size:64;not null"`
	OrderID      int64     `json:"order_id,string" gorm:"not null;index:idx_return_tenant_order,priority:2"`
	OrderNo      string    `json:"order_no" gorm:"size:64;not null"`
	AllocationID int64     `json:"allocation_id,string" gorm:"not null;index"`
	WarehouseID  int64     `json:"warehouse_id,string" gorm:"not null"`
	LocationID   int64     `json:"location_id,string" gorm:"not null"`
	SKUID        int64     `json:"sku_id,string" gorm:"column:sku_id;not null"`
	BatchNo      string    `json:"batch_no" gorm:"size:64;not null"`
	GoodQty      int       `json:"good_qty" gorm:"type:int;size:32;not null"`
	BadQty       int       `json:"bad_qty" gorm:"type:int;size:32;not null"`
	Reason       string    `json:"reason" gorm:"size:255;not null"`
	Operator     string    `json:"operator" gorm:"size:64;not null"`
	CreatedAt    time.Time `json:"created_at"`
}

func (CustomerReturn) TableName() string { return "wms_customer_return" }
