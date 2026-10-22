package dto

type ExportQuery struct {
	WarehouseID     int64  `form:"warehouse_id" binding:"gte=0"`
	OrderID         int64  `form:"order_id" binding:"gte=0"`
	Status          string `form:"status" binding:"omitempty,oneof=DRAFT COMPLETED CANCELLED"`
	Keyword         string `form:"keyword" binding:"max=64"`
	DifferencesOnly bool   `form:"differences_only"`
}
