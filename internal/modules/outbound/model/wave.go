package model

import (
	"time"

	"gowms/internal/pkg/modelbase"
)

type WaveStatus string

const (
	WaveCreated   WaveStatus = "CREATED"
	WavePicking   WaveStatus = "PICKING"
	WaveSorting   WaveStatus = "SORTING"
	WaveReady     WaveStatus = "READY"
	WaveCompleted WaveStatus = "COMPLETED"
	WaveCancelled WaveStatus = "CANCELLED"
)

// Wave serializes its bounded picking/sorting work under the header row lock.
// Picked goods remain reserved against their source inventory until explicit shipment.
type Wave struct {
	modelbase.Base
	TenantID      int64      `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_wave_no,priority:1;index:idx_wave_tenant"`
	WaveNo        string     `json:"wave_no" gorm:"size:64;not null;uniqueIndex:uk_wave_no,priority:2"`
	WarehouseID   int64      `json:"warehouse_id,string" gorm:"not null;index"`
	Status        WaveStatus `json:"status" gorm:"size:16;not null;default:CREATED"`
	Remark        string     `json:"remark" gorm:"size:255"`
	CreatedBy     string     `json:"created_by" gorm:"size:64"`
	ClaimedBy     string     `json:"claimed_by" gorm:"size:64;not null;default:''"`
	LeaseExpireAt *time.Time `json:"lease_expire_at"`
}

func (Wave) TableName() string { return "wms_wave" }

type WaveOrder struct {
	modelbase.Base
	TenantID      int64      `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_wave_active_order,priority:1"`
	WaveID        int64      `json:"wave_id,string" gorm:"not null;index"`
	OrderID       int64      `json:"order_id,string" gorm:"not null;index"`
	ActiveOrderID *int64     `json:"-" gorm:"uniqueIndex:uk_wave_active_order,priority:2"`
	OrderNo       string     `json:"order_no" gorm:"size:64;not null"`
	SlotCode      string     `json:"slot_code" gorm:"size:16;not null"`
	ShippedAt     *time.Time `json:"shipped_at"`
}

func (WaveOrder) TableName() string { return "wms_wave_order" }

// WaveLine groups only the same source inventory (SKU, bin and batch stay distinct).
type WaveLine struct {
	modelbase.Base
	TenantID     int64  `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_wave_inventory,priority:1"`
	WaveID       int64  `json:"wave_id,string" gorm:"not null;index;uniqueIndex:uk_wave_inventory,priority:2"`
	InventoryID  int64  `json:"inventory_id,string" gorm:"not null;uniqueIndex:uk_wave_inventory,priority:3"`
	SKUID        int64  `json:"sku_id,string" gorm:"column:sku_id;not null"`
	SKUCode      string `json:"sku_code" gorm:"size:64"`
	Barcode      string `json:"barcode" gorm:"size:64"`
	SKUName      string `json:"sku_name" gorm:"size:128"`
	LocationCode string `json:"location_code" gorm:"size:64"`
	BatchNo      string `json:"batch_no" gorm:"size:64"`
	TargetQty    int64  `json:"target_qty" gorm:"not null;check:chk_wave_line_qty,target_qty > 0 AND picked_qty >= 0 AND picked_qty <= target_qty"`
	PickedQty    int64  `json:"picked_qty" gorm:"not null;default:0"`
}

func (WaveLine) TableName() string { return "wms_wave_line" }

// WaveItem preserves the original task/allocation and the destination order slot.
type WaveItem struct {
	modelbase.Base
	TenantID     int64 `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_wave_task,priority:1"`
	WaveID       int64 `json:"wave_id,string" gorm:"not null;index;uniqueIndex:uk_wave_task,priority:2"`
	WaveOrderID  int64 `json:"wave_order_id,string" gorm:"not null;index"`
	LineID       int64 `json:"line_id,string" gorm:"not null;index"`
	TaskID       int64 `json:"task_id,string" gorm:"not null;uniqueIndex:uk_wave_task,priority:3"`
	AllocationID int64 `json:"allocation_id,string" gorm:"not null"`
	TargetQty    int   `json:"target_qty" gorm:"type:int;size:32;not null;check:chk_wave_item_qty,target_qty > 0 AND sorted_qty >= 0 AND sorted_qty <= target_qty"`
	SortedQty    int   `json:"sorted_qty" gorm:"type:int;size:32;not null;default:0"`
}

func (WaveItem) TableName() string { return "wms_wave_item" }
