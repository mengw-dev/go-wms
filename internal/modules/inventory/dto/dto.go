// Package dto 定义库存查询接口的请求和响应结构。
package dto

import "time"

type InventoryQuery struct {
	WarehouseID int64  `form:"warehouse_id"`
	LocationID  int64  `form:"location_id"`
	SKUID       int64  `form:"sku_id"`
	SKUKeyword  string `form:"sku_keyword"`
	// InStockOnly 为 true 时只返回现存量大于 0 的库存行。
	InStockOnly bool `form:"in_stock_only"`
	Page        int  `form:"page,default=1" binding:"min=1"`
	PageSize    int  `form:"page_size,default=10" binding:"min=1,max=100"`
}

type SummaryQuery struct {
	WarehouseID int64 `form:"warehouse_id"`
	Page        int   `form:"page,default=1" binding:"min=1"`
	PageSize    int   `form:"page_size,default=10" binding:"min=1,max=100"`
}

type TransQuery struct {
	InventoryID int64  `form:"inventory_id"`
	OrderNo     string `form:"order_no"`
	TransType   string `form:"trans_type"` // RECEIVE/ALLOCATE/SHIP/RELEASE/ADJUST
	Page        int    `form:"page,default=1" binding:"min=1"`
	PageSize    int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

// InventoryResp 是库存列表的稳定响应契约，不直接暴露 GORM Model。
type InventoryResp struct {
	ID            int64     `json:"id,string"`
	TenantID      int64     `json:"tenant_id,string"`
	WarehouseID   int64     `json:"warehouse_id,string"`
	LocationID    int64     `json:"location_id,string"`
	SKUID         int64     `json:"sku_id,string"`
	BatchNo       string    `json:"batch_no"`
	StockQuantity int       `json:"stock_quantity"`
	AvailableQty  int       `json:"available_quantity"`
	AllocatedQty  int       `json:"allocated_quantity"`
	StockInTime   time.Time `json:"stock_in_time"`
	LocationCode  string    `json:"location_code"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// InventoryTransResp 是库存流水列表的稳定响应契约。
type InventoryTransResp struct {
	ID              int64     `json:"id,string"`
	TenantID        int64     `json:"tenant_id,string"`
	InventoryID     int64     `json:"inventory_id,string"`
	TransType       string    `json:"trans_type"`
	QuantityChange  int       `json:"quantity_change"`
	BeforeQuantity  int       `json:"before_quantity"`
	AfterQuantity   int       `json:"after_quantity"`
	AvailableBefore int       `json:"available_before"`
	AvailableAfter  int       `json:"available_after"`
	OrderNo         string    `json:"order_no"`
	TaskNo          string    `json:"task_no"`
	Operator        string    `json:"operator"`
	CreatedAt       time.Time `json:"created_at"`
}
