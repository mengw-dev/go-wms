// Package dto 定义入库接口的请求和响应结构。
package dto

import (
	"time"

	"gowms/internal/modules/inbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/typex"
)

type OrderDetailItem struct {
	SKUID       int64 `json:"sku_id,string" binding:"required"`
	ExpectedQty int   `json:"expected_qty" binding:"required,min=1"`
}

type CreateOrderReq struct {
	WarehouseID int64             `json:"warehouse_id,string" binding:"required"`
	Remark      string            `json:"remark" binding:"max=255"`
	Details     []OrderDetailItem `json:"details" binding:"required,min=1,dive"`
}

type OrderQuery struct {
	WarehouseID   int64  `form:"warehouse_id"`
	Status        string `form:"status"`
	Keyword       string `form:"keyword"`
	ImportTaskID  string `form:"import_task_id"`
	CreatedAtFrom string `form:"created_at_from"`
	CreatedAtTo   string `form:"created_at_to"`
	Page          int    `form:"page,default=1" binding:"min=1"`
	PageSize      int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type ReceiveReq struct {
	DetailID     int64  `json:"detail_id,string" binding:"required"`
	Qty          int    `json:"qty" binding:"required,min=1"`  // 本次收货总量，包含残品
	DefectiveQty int    `json:"defective_qty" binding:"min=0"` // 总量中的残品，不能大于 Qty
	BatchNo      string `json:"batch_no" binding:"max=64"`
}

type PutawayReq struct {
	LocationID int64 `json:"location_id,string" binding:"required"`
	Qty        int   `json:"qty" binding:"required,min=1"`
}

type ImportResp struct {
	TaskID string `json:"task_id"`
}

// BatchOperReq 批量操作请求，IDs 兼容 JSON 数字数组和字符串数组（typex.Int64List）。
type BatchOperReq struct {
	IDs typex.Int64List `json:"ids" binding:"required,min=1,max=200"`
}

// BatchItemError 表示批量操作中处理失败的一项。
type BatchItemError struct {
	ID  int64  `json:"id,string"`
	Msg string `json:"msg"`
}

type BatchOperResp struct {
	Success int              `json:"success"`
	Fail    int              `json:"fail"`
	Errors  []BatchItemError `json:"errors,omitempty"`
}

// ImportTaskResp 是导入任务查询接口的稳定响应契约，不暴露执行 token 和文件路径。
type ImportTaskResp struct {
	ID          int64     `json:"id,string"`
	TenantID    int64     `json:"tenant_id,string"`
	TaskID      string    `json:"task_id"`
	Status      string    `json:"status"`
	FileName    string    `json:"file_name"`
	TotalRows   int       `json:"total_rows"`
	SuccessRows int       `json:"success_rows"`
	FailRows    int       `json:"fail_rows"`
	ErrorMsg    string    `json:"error_msg"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// OrderResp 是入库单列表和创建接口的稳定响应契约。
type OrderResp struct {
	ID           int64             `json:"id,string"`
	TenantID     int64             `json:"tenant_id,string"`
	OrderNo      string            `json:"order_no"`
	WarehouseID  int64             `json:"warehouse_id,string"`
	Status       model.OrderStatus `json:"status"`
	Source       string            `json:"source"`
	Remark       string            `json:"remark"`
	ExpectedQty  int               `json:"expected_qty"`
	ReceivedQty  int               `json:"received_qty"`
	DefectiveQty int               `json:"defective_qty"`
	ImportTaskID *string           `json:"import_task_id"`
	ImportRow    int               `json:"import_row"`
	CreatedBy    string            `json:"created_by"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
}
type OrderDetailResp struct {
	Order   *OrderResp            `json:"order"`
	Details []*OrderDetailRowResp `json:"details"`
	Tasks   []*OrderTaskResp      `json:"tasks"`
}

type OrderDetailRowResp struct {
	ID           int64     `json:"id,string"`
	TenantID     int64     `json:"tenant_id,string"`
	OrderID      int64     `json:"order_id,string"`
	SKUID        int64     `json:"sku_id,string"`
	SKUCode      string    `json:"sku_code"`
	SKUName      string    `json:"sku_name"`
	ExpectedQty  int       `json:"expected_qty"`
	ReceivedQty  int       `json:"received_qty"`
	DefectiveQty int       `json:"defective_qty"`
	BatchNo      string    `json:"batch_no"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
