package dto

import "time"

type MoveSource struct {
	ID            int64     `json:"id,string"`
	WarehouseID   int64     `json:"warehouse_id,string"`
	WarehouseCode string    `json:"warehouse_code"`
	LocationCode  string    `json:"location_code"`
	SKUCode       string    `json:"sku_code"`
	SKUName       string    `json:"sku_name"`
	Unit          string    `json:"unit"`
	BatchNo       string    `json:"batch_no"`
	AvailableQty  int       `json:"available_qty"`
	StockInTime   time.Time `json:"stock_in_time"`
	Enabled       bool      `json:"enabled"`
}

type MoveRequest struct {
	SourceInventoryID int64  `json:"source_inventory_id,string" binding:"required,gt=0"`
	TargetLocationID  int64  `json:"target_location_id,string" binding:"required,gt=0"`
	Quantity          int    `json:"quantity" binding:"required,gt=0,lte=2147483647"`
	Reason            string `json:"reason" binding:"required,max=255"`
}

type MoveQuery struct {
	WarehouseID  int64  `form:"warehouse_id" binding:"gte=0"`
	Keyword      string `form:"keyword" binding:"max=128"`
	OperationKey string `form:"operation_key" binding:"max=64"`
	Page         int    `form:"page,default=1" binding:"gte=1"`
	PageSize     int    `form:"page_size,default=10" binding:"gte=1,lte=100"`
}

type Movement struct {
	ID                 int64     `json:"id,string"`
	MoveNo             string    `json:"move_no"`
	WarehouseID        int64     `json:"warehouse_id,string"`
	WarehouseCode      string    `json:"warehouse_code"`
	SourceInventoryID  int64     `json:"source_inventory_id,string"`
	TargetInventoryID  int64     `json:"target_inventory_id,string"`
	SourceLocationID   int64     `json:"source_location_id,string"`
	TargetLocationID   int64     `json:"target_location_id,string"`
	SourceLocationCode string    `json:"source_location_code"`
	TargetLocationCode string    `json:"target_location_code"`
	SKUID              int64     `json:"sku_id,string"`
	SKUCode            string    `json:"sku_code"`
	SKUName            string    `json:"sku_name"`
	Unit               string    `json:"unit"`
	BatchNo            string    `json:"batch_no"`
	Quantity           int       `json:"quantity"`
	StockInTime        time.Time `json:"stock_in_time"`
	Operator           string    `json:"operator"`
	Reason             string    `json:"reason"`
	CreatedAt          time.Time `json:"created_at"`
}

type MoveLocationQuery struct {
	SourceInventoryID int64  `form:"source_inventory_id" binding:"required,gt=0"`
	Keyword           string `form:"keyword" binding:"max=64"`
	Page              int    `form:"page,default=1" binding:"gte=1"`
	PageSize          int    `form:"page_size,default=30" binding:"gte=1,lte=100"`
}
