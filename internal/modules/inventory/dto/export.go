package dto

// InventoryExportQuery omits pagination: all matching rows are exported or the request fails.
type InventoryExportQuery struct {
	WarehouseID int64  `form:"warehouse_id" binding:"gte=0"`
	LocationID  int64  `form:"location_id" binding:"gte=0"`
	SKUID       int64  `form:"sku_id" binding:"gte=0"`
	SKUKeyword  string `form:"sku_keyword" binding:"max=128"`
	InStockOnly bool   `form:"in_stock_only"`
}

type TransExportQuery struct {
	InventoryID int64  `form:"inventory_id" binding:"gte=0"`
	OrderNo     string `form:"order_no" binding:"max=64"`
	TransType   string `form:"trans_type" binding:"omitempty,oneof=OPENING RECEIVE ALLOCATE SHIP RELEASE ADJUST MOVE_IN MOVE_OUT"`
}
