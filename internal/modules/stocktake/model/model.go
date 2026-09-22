package model

import (
	"gowms/internal/modules/system/model"
)

// OrderStatus 盘点单状态机：DRAFT（快照/录入实盘）→ COMPLETED（审核调整）；可 CANCELLED。
type OrderStatus string

const (
	OrderDraft     OrderStatus = "DRAFT"
	OrderCompleted OrderStatus = "COMPLETED"
	OrderCancelled OrderStatus = "CANCELLED"
)

// StatusTransitions 盘点单状态转换表：草稿可完成或取消，终态无后继。
var StatusTransitions = map[OrderStatus][]OrderStatus{
	OrderDraft: {OrderCompleted, OrderCancelled},
}

// CanTransit 判断盘点单状态转换是否合法。
func CanTransit(from, to OrderStatus) bool {
	for _, next := range StatusTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

type StocktakeOrder struct {
	model.Base
	model.Versioned
	TenantID     int64       `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_stocktake_no,priority:1;index:idx_sto_tenant"`
	OrderNo      string      `json:"order_no" gorm:"size:64;uniqueIndex:uk_stocktake_no,priority:2;not null"`
	WarehouseID  int64       `json:"warehouse_id,string" gorm:"not null"`
	LocationID   int64       `json:"location_id,string"` // 0 表示整仓盘点
	LocationCode string      `json:"location_code" gorm:"size:64"`
	Status       OrderStatus `json:"status" gorm:"size:16;index;not null;default:'DRAFT'"`
	Remark       string      `json:"remark" gorm:"size:255"`
	CreatedBy    string      `json:"created_by" gorm:"size:64"`
}

func (StocktakeOrder) TableName() string { return "wms_stocktake_order" }

type StocktakeDetail struct {
	model.Base
	TenantID     int64  `json:"tenant_id,string" gorm:"not null;default:0"`
	OrderID      int64  `json:"order_id,string" gorm:"index;not null"`
	InventoryID  int64  `json:"inventory_id,string" gorm:"not null"`
	SKUID        int64  `json:"sku_id,string" gorm:"column:sku_id;not null"`
	SKUCode      string `json:"sku_code" gorm:"size:64"`
	SKUName      string `json:"sku_name" gorm:"size:128"`
	LocationID   int64  `json:"location_id,string"`
	LocationCode string `json:"location_code" gorm:"size:64"`
	BatchNo      string `json:"batch_no" gorm:"size:64"`
	BookQty      int    `json:"book_qty"`   // 快照账面库存
	ActualQty    *int   `json:"actual_qty"` // 实盘数，nil 表示未盘
	DiffQty      int    `json:"diff_qty"`   // actual - 当前库存，审核时重算
	Adjusted     bool   `json:"adjusted"`
}

func (StocktakeDetail) TableName() string { return "wms_stocktake_detail" }
