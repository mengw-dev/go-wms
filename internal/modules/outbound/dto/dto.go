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
	TaskID int64 `json:"task_id,string" binding:"required"`
	Qty    int   `json:"qty" binding:"required,min=1"`
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

// 批量操作请求。IDs 兼容 JSON 数字数组和字符串数组（typex.Int64List）。
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
