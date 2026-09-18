package dto

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
