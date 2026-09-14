package model

import (
	"gowms/internal/modules/system/model"
)

// OrderStatus 出库单状态机：DRAFT → SUBMITTED → APPROVED(即分配) → PICKING → SHIPPED（可 CANCELLED）。
type OrderStatus string

const (
	OrderDraft     OrderStatus = "DRAFT"
	OrderSubmitted OrderStatus = "SUBMITTED"
	OrderApproved  OrderStatus = "APPROVED"
	OrderPicking   OrderStatus = "PICKING"
	OrderShipped   OrderStatus = "SHIPPED"
	OrderCancelled OrderStatus = "CANCELLED"
)

// OrderNoPrefix 出库单号前缀。
const OrderNoPrefix = "CK"

// StatusTransitions 状态转换表。
// 审核即分配，SUBMITTED 审核通过后直接进入 PICKING（不停留 APPROVED）；
// APPROVED 行保留兼容历史数据；终态 SHIPPED/CANCELLED 无后继。
var StatusTransitions = map[OrderStatus][]OrderStatus{
	OrderDraft:     {OrderSubmitted, OrderCancelled},
	OrderSubmitted: {OrderPicking, OrderCancelled},
	OrderApproved:  {OrderPicking, OrderCancelled},
	OrderPicking:   {OrderShipped, OrderCancelled},
}

// CanTransit 判断状态是否允许从 from 流转到 to，是状态机校验的唯一入口。
func CanTransit(from, to OrderStatus) bool {
	for _, next := range StatusTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

type ShipmentOrder struct {
	model.Base
	model.Versioned
	OrderNo      string      `json:"order_no" gorm:"size:64;uniqueIndex;not null"`
	BizOrderNo   string      `json:"biz_order_no" gorm:"size:64;uniqueIndex;not null"` // 幂等键：业务订单号
	WarehouseID  int64       `json:"warehouse_id,string" gorm:"not null"`
	Status       OrderStatus `json:"status" gorm:"size:16;index;not null;default:'DRAFT'"`
	Remark       string      `json:"remark" gorm:"size:255"`
	ExpectedQty  int         `json:"expected_qty" gorm:"not null;default:0"`
	AllocatedQty int         `json:"allocated_qty" gorm:"not null;default:0"`
	PickedQty    int         `json:"picked_qty" gorm:"not null;default:0"`
	CreatedBy    string      `json:"created_by" gorm:"size:64"`
}

func (ShipmentOrder) TableName() string { return "wms_shipment_order" }

type ShipmentOrderDetail struct {
	model.Base
	OrderID      int64  `json:"order_id,string" gorm:"index;not null"`
	SKUID        int64  `json:"sku_id,string" gorm:"column:sku_id;not null"`
	SKUCode      string `json:"sku_code" gorm:"size:64"`
	SKUName      string `json:"sku_name" gorm:"size:128"`
	ExpectedQty  int    `json:"expected_qty" gorm:"not null"`
	AllocatedQty int    `json:"allocated_qty" gorm:"not null;default:0"`
	PickedQty    int    `json:"picked_qty" gorm:"not null;default:0"`
}

func (ShipmentOrderDetail) TableName() string { return "wms_shipment_order_detail" }

// AllocationStatus 分配行状态。
type AllocationStatus string

const (
	AllocAllocated AllocationStatus = "ALLOCATED"
	AllocPicked    AllocationStatus = "PICKED"
	AllocCancelled AllocationStatus = "CANCELLED"
)

// Allocation 分配明细：FIFO 分配会跨批次/库位，一行出库明细对应 N 个分配行。
// 拣货任务、发货扣减、取消释放均以本表为准。
type Allocation struct {
	model.Base
	model.Versioned
	OrderID      int64            `json:"order_id,string" gorm:"index;not null"`
	DetailID     int64            `json:"detail_id,string" gorm:"index;not null"`
	InventoryID  int64            `json:"inventory_id,string" gorm:"index;not null"`
	SKUID        int64            `json:"sku_id,string" gorm:"column:sku_id;not null"`
	LocationID   int64            `json:"location_id,string" gorm:"not null"`
	LocationCode string           `json:"location_code" gorm:"size:64"`
	BatchNo      string           `json:"batch_no" gorm:"size:64"`
	AllocatedQty int              `json:"allocated_qty" gorm:"not null"`
	PickedQty    int              `json:"picked_qty" gorm:"not null;default:0"`
	Status       AllocationStatus `json:"status" gorm:"size:16;index;not null;default:'ALLOCATED'"`
}

func (Allocation) TableName() string { return "wms_allocation" }
