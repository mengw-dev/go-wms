package dto

import "time"

type AlertQuery struct {
	WarehouseID int64  `form:"warehouse_id" binding:"gte=0"`
	SKUKeyword  string `form:"sku_keyword" binding:"max=100"`
	State       string `form:"state,default=ALERT" binding:"oneof=ALERT ALL LOW OUT_OF_STOCK NORMAL PAUSED UNAVAILABLE"`
	Page        int    `form:"page,default=1" binding:"min=1"`
	PageSize    int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type SaveAlertRuleReq struct {
	WarehouseID     int64 `json:"warehouse_id,string" binding:"required,gt=0"`
	SKUID           int64 `json:"sku_id,string" binding:"required,gt=0"`
	MinAvailableQty int   `json:"min_available_qty" binding:"required,min=1,max=2147483647"`
	Enabled         *bool `json:"enabled" binding:"required"`
}

type AlertRuleSaved struct {
	ID int64 `json:"id,string"`
}

type AlertRow struct {
	ID              int64     `json:"id,string"`
	WarehouseID     int64     `json:"warehouse_id,string"`
	WarehouseCode   string    `json:"warehouse_code"`
	WarehouseName   string    `json:"warehouse_name"`
	SKUID           int64     `json:"sku_id,string" gorm:"column:sku_id"`
	SKUCode         string    `json:"sku_code"`
	SKUName         string    `json:"sku_name"`
	Unit            string    `json:"unit"`
	MinAvailableQty int       `json:"min_available_qty"`
	Enabled         bool      `json:"enabled"`
	StockQuantity   int64     `json:"stock_quantity"`
	AvailableQty    int64     `json:"available_qty"`
	AllocatedQty    int64     `json:"allocated_qty"`
	GapQty          int64     `json:"gap_qty"`
	State           string    `json:"state"`
	UpdatedBy       string    `json:"updated_by"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type AlertSummary struct {
	Configured int64 `json:"configured"`
	OutOfStock int64 `json:"out_of_stock"`
	Low        int64 `json:"low"`
}

type AlertListResp struct {
	Warehouses []*AlertOption `json:"warehouses"`
	List       []*AlertRow    `json:"list"`
	Total      int64          `json:"total"`
	Summary    AlertSummary   `json:"summary"`
	CheckedAt  time.Time      `json:"checked_at"`
}

type AlertOption struct {
	ID   int64  `json:"id,string"`
	Code string `json:"code"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type AlertOptionQuery struct {
	Kind     string `form:"kind" binding:"required,oneof=warehouse sku"`
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=100" binding:"min=1,max=100"`
}
