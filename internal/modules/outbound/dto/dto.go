package dto

import (
	"time"

	"gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/typex"
)

type OrderDetailItem struct {
	SKUID       int64 `json:"sku_id,string" binding:"required"`
	ExpectedQty int   `json:"expected_qty" binding:"required,min=1"`
}

type CreateOrderReq struct {
	WarehouseID int64             `json:"warehouse_id,string" binding:"required"`
	BizOrderNo  string            `json:"biz_order_no" binding:"required,max=64"`
	Remark      string            `json:"remark" binding:"max=255"`
	Details     []OrderDetailItem `json:"details" binding:"required,min=1,dive"`
}

type OrderQuery struct {
	WarehouseID   int64  `form:"warehouse_id"`
	Status        string `form:"status"`
	Keyword       string `form:"keyword"`
	CreatedAtFrom string `form:"created_at_from"`
	CreatedAtTo   string `form:"created_at_to"`
	Page          int    `form:"page,default=1" binding:"min=1"`
	PageSize      int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type PickReq struct {
	Qty int `json:"qty" binding:"required,min=1"`
	// 扫码核对字段（可选）：填写时后端校验必须与任务要求的库位/批次一致。
	LocationCode string `json:"location_code"`
	BatchNo      string `json:"batch_no"`
}

type ExternalOrderDetailItem struct {
	SKUCode     string `json:"sku_code" binding:"required,max=64"`
	ExpectedQty int    `json:"expected_qty" binding:"required,min=1"`
}

type ExternalCreateOrderReq struct {
	WarehouseCode string                    `json:"warehouse_code" binding:"required,max=32"`
	BizOrderNo    string                    `json:"biz_order_no" binding:"required,max=64"`
	Remark        string                    `json:"remark" binding:"max=255"`
	Details       []ExternalOrderDetailItem `json:"details" binding:"required,min=1,dive"`
}

type ExternalCreateOrderResp struct {
	OrderID    int64  `json:"order_id,string"`
	OrderNo    string `json:"order_no"`
	BizOrderNo string `json:"biz_order_no"`
	Status     string `json:"status"`
	Idempotent bool   `json:"idempotent"`
}

// BatchOperReq 批量操作请求，IDs 兼容 JSON 数字数组和字符串数组（typex.Int64List）。
type BatchOperReq struct {
	IDs typex.Int64List `json:"ids" binding:"required,min=1,max=200"`
}

type BatchItemError struct {
	ID  int64  `json:"id,string"`
	Msg string `json:"msg"`
}

type BatchOperResp struct {
	Success int              `json:"success"`
	Fail    int              `json:"fail"`
	Errors  []BatchItemError `json:"errors,omitempty"`
}

// OrderResp 是出库单列表和创建接口的稳定响应契约。
type OrderResp struct {
	ID           int64             `json:"id,string"`
	TenantID     int64             `json:"tenant_id,string"`
	OrderNo      string            `json:"order_no"`
	BizOrderNo   string            `json:"biz_order_no"`
	WarehouseID  int64             `json:"warehouse_id,string"`
	Status       model.OrderStatus `json:"status"`
	Remark       string            `json:"remark"`
	ExpectedQty  int               `json:"expected_qty"`
	AllocatedQty int               `json:"allocated_qty"`
	PickedQty    int               `json:"picked_qty"`
	CreatedBy    string            `json:"created_by"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
type OrderDetailResp struct {
	Order       *OrderResp            `json:"order"`
	Details     []*OrderDetailRowResp `json:"details"`
	Allocations []*AllocationResp     `json:"allocations"`
	Tasks       []*OrderTaskResp      `json:"tasks"`
}

type OrderDetailRowResp struct {
	ID           int64     `json:"id,string"`
	TenantID     int64     `json:"tenant_id,string"`
	OrderID      int64     `json:"order_id,string"`
	SKUID        int64     `json:"sku_id,string"`
	SKUCode      string    `json:"sku_code"`
	SKUName      string    `json:"sku_name"`
	ExpectedQty  int       `json:"expected_qty"`
	AllocatedQty int       `json:"allocated_qty"`
	PickedQty    int       `json:"picked_qty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type AllocationResp struct {
	ID           int64                  `json:"id,string"`
	TenantID     int64                  `json:"tenant_id,string"`
	OrderID      int64                  `json:"order_id,string"`
	DetailID     int64                  `json:"detail_id,string"`
	InventoryID  int64                  `json:"inventory_id,string"`
	SKUID        int64                  `json:"sku_id,string"`
	LocationID   int64                  `json:"location_id,string"`
	LocationCode string                 `json:"location_code"`
	BatchNo      string                 `json:"batch_no"`
	AllocatedQty int                    `json:"allocated_qty"`
	PickedQty    int                    `json:"picked_qty"`
	Status       model.AllocationStatus `json:"status"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type OrderTaskResp struct {
	ID           int64                `json:"id,string"`
	TenantID     int64                `json:"tenant_id,string"`
	TaskNo       string               `json:"task_no"`
	TaskType     taskmodel.TaskType   `json:"task_type"`
	Status       taskmodel.TaskStatus `json:"status"`
	OrderID      int64                `json:"order_id,string"`
	OrderNo      string               `json:"order_no"`
	DetailID     int64                `json:"detail_id,string"`
	AllocationID int64                `json:"allocation_id,string"`
	SKUID        int64                `json:"sku_id,string"`
	WarehouseID  int64                `json:"warehouse_id,string"`
	LocationID   int64                `json:"location_id,string"`
	LocationCode string               `json:"location_code"`
	BatchNo      string               `json:"batch_no"`
	TargetQty    int                  `json:"target_qty"`
	DoneQty      int                  `json:"done_qty"`
	Operator     string               `json:"operator"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}
