package model

import (
	"time"

	"gowms/internal/modules/system/model"
)

// Inventory 库存：仓库+库位+SKU+批次 四元组唯一（tenant_id 前置联合唯一，各租户独立）。
// 三数量模型：stock_quantity = available_quantity + allocated_quantity。
type Inventory struct {
	model.Base
	model.Versioned           // 乐观锁，配合条件更新防并发
	TenantID       int64      `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_inv,priority:1;index:idx_inv_tenant"`
	WarehouseID    int64      `json:"warehouse_id,string" gorm:"not null;uniqueIndex:uk_inv,priority:2"`
	LocationID     int64      `json:"location_id,string" gorm:"not null;uniqueIndex:uk_inv,priority:3"`
	SKUID          int64      `json:"sku_id,string" gorm:"column:sku_id;not null;uniqueIndex:uk_inv,priority:4"`
	BatchNo        string     `json:"batch_no" gorm:"size:64;not null;default:'';uniqueIndex:uk_inv,priority:5"`
	StockQuantity  int        `json:"stock_quantity" gorm:"not null;default:0"`
	AvailableQty   int        `json:"available_quantity" gorm:"column:available_quantity;not null;default:0"`
	AllocatedQty   int        `json:"allocated_quantity" gorm:"column:allocated_quantity;not null;default:0"`
	StockInTime    time.Time  `json:"stock_in_time"`           // FIFO 依据
	LocationCode   string     `json:"location_code" gorm:"->"` // 联查字段，不落库
}

func (Inventory) TableName() string { return "wms_inventory" }

// TransType 流水类型。
type TransType string

const (
	TransReceive  TransType = "RECEIVE"  // 上架入库 +
	TransAllocate TransType = "ALLOCATE" // 分配锁定：available- allocated+
	TransShip     TransType = "SHIP"     // 发货扣减：stock- allocated-
	TransRelease  TransType = "RELEASE"  // 取消释放：available+ allocated-
	TransAdjust   TransType = "ADJUST"   // 盘点调整
)

// InventoryTrans 库存流水：只增不改，每次变动同事务写入。
type InventoryTrans struct {
	ID              int64     `json:"id,string" gorm:"primaryKey"`
	TenantID        int64     `json:"tenant_id,string" gorm:"not null;default:0;index:idx_trans_tenant"`
	InventoryID     int64     `json:"inventory_id,string" gorm:"index"`
	TransType       TransType `json:"trans_type" gorm:"size:16;index"`
	QuantityChange  int       `json:"quantity_change"`
	BeforeQuantity  int       `json:"before_quantity"` // 变更前 stock_quantity
	AfterQuantity   int       `json:"after_quantity"`  // 变更后 stock_quantity
	AvailableBefore int       `json:"available_before"`
	AvailableAfter  int       `json:"available_after"`
	OrderNo         string    `json:"order_no" gorm:"size:64;index"` // 来源单据
	TaskNo          string    `json:"task_no" gorm:"size:64"`
	Operator        string    `json:"operator" gorm:"size:64"`
	CreatedAt       time.Time `json:"created_at" gorm:"index"`
}

func (InventoryTrans) TableName() string { return "wms_inventory_trans" }
