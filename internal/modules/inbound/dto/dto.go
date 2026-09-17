package dto

import (
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
	Qty          int    `json:"qty" binding:"required,min=1"`
	DefectiveQty int    `json:"defective_qty" binding:"min=0"`
	BatchNo      string `json:"batch_no" binding:"max=64"`
}

type PutawayReq struct {
	TaskID     int64 `json:"task_id,string" binding:"required"`
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
