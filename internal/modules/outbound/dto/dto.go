package dto

type OrderDetailItem struct {
	SKUID       int64 `json:"sku_id,string" binding:"required"`
	ExpectedQty int   `json:"expected_qty" binding:"required,min=1"`
}

type CreateOrderReq struct {
	WarehouseID int64             `json:"warehouse_id,string" binding:"required"`
	BizOrderNo  string            `json:"biz_order_no" binding:"required,max=64"` // 幂等键
	Remark      string            `json:"remark" binding:"max=255"`
	Details     []OrderDetailItem `json:"details" binding:"required,min=1,dive"`
}

type OrderQuery struct {
	WarehouseID int64  `form:"warehouse_id"`
	Status      string `form:"status"`
	Keyword     string `form:"keyword"` // 出库单号/业务单号
	Page        int    `form:"page,default=1" binding:"min=1"`
	PageSize    int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type PickReq struct {
	TaskID int64 `json:"task_id,string" binding:"required"`
	Qty    int   `json:"qty" binding:"required,min=1"`
}

// ExternalOrderDetailItem 外部系统按货品编码推送的出库明细。
type ExternalOrderDetailItem struct {
	SKUCode     string `json:"sku_code" binding:"required,max=64"`
	ExpectedQty int    `json:"expected_qty" binding:"required,min=1"`
}

// ExternalCreateOrderReq OMS/ERP 等外部系统推送的出库单请求。
type ExternalCreateOrderReq struct {
	WarehouseCode string                    `json:"warehouse_code" binding:"required,max=32"`
	BizOrderNo    string                    `json:"biz_order_no" binding:"required,max=64"`
	Remark        string                    `json:"remark" binding:"max=255"`
	Details       []ExternalOrderDetailItem `json:"details" binding:"required,min=1,dive"`
}

// ExternalCreateOrderResp 外部系统推送成功后的稳定响应。
type ExternalCreateOrderResp struct {
	OrderID    int64  `json:"order_id,string"`
	OrderNo    string `json:"order_no"`
	BizOrderNo string `json:"biz_order_no"`
	Status     string `json:"status"`
	Idempotent bool   `json:"idempotent"`
}
