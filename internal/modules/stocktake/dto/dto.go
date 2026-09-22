// Package dto 定义盘点接口的请求和响应结构。
package dto

import "time"

type CreateOrderReq struct {
	WarehouseID  int64  `json:"warehouse_id,string" binding:"required"`
	LocationID   int64  `json:"location_id,string"` // 0 = 整仓
	LocationCode string `json:"location_code" binding:"max=64"`
	Remark       string `json:"remark" binding:"max=255"`
}

type OrderQuery struct {
	WarehouseID int64  `form:"warehouse_id"`
	Status      string `form:"status"`
	Page        int    `form:"page,default=1" binding:"min=1"`
	PageSize    int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type RecordActualReq struct {
	DetailID  int64 `json:"detail_id,string" binding:"required"`
	ActualQty int   `json:"actual_qty" binding:"min=0"`
}

// OrderResp 是盘点单列表和创建接口的稳定响应契约。
type OrderResp struct {
	ID           int64     `json:"id,string"`
	TenantID     int64     `json:"tenant_id,string"`
	OrderNo      string    `json:"order_no"`
	WarehouseID  int64     `json:"warehouse_id,string"`
	LocationID   int64     `json:"location_id,string"`
	LocationCode string    `json:"location_code"`
	Status       string    `json:"status"`
	Remark       string    `json:"remark"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// DetailResp 是盘点明细的稳定响应契约。
type DetailResp struct {
	ID           int64     `json:"id,string"`
	TenantID     int64     `json:"tenant_id,string"`
	OrderID      int64     `json:"order_id,string"`
	InventoryID  int64     `json:"inventory_id,string"`
	SKUID        int64     `json:"sku_id,string"`
	SKUCode      string    `json:"sku_code"`
	SKUName      string    `json:"sku_name"`
	LocationID   int64     `json:"location_id,string"`
	LocationCode string    `json:"location_code"`
	BatchNo      string    `json:"batch_no"`
	BookQty      int       `json:"book_qty"`
	ActualQty    *int      `json:"actual_qty"`
	DiffQty      int       `json:"diff_qty"`
	Adjusted     bool      `json:"adjusted"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// OrderDetailResp 是盘点单详情接口的稳定响应契约。
type OrderDetailResp struct {
	Order   *OrderResp    `json:"order"`
	Details []*DetailResp `json:"details"`
}
