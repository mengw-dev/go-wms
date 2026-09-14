package dto

type CreateOrderReq struct {
	WarehouseID  int64  `json:"warehouse_id,string" binding:"required"`
	LocationID   int64  `json:"location_id,string"` // 0 = 整仓
	LocationCode string `json:"location_code" binding:"max=64"`
	Remark       string `json:"remark" binding:"max=255"`
}

type OrderQuery struct {
	WarehouseID int64  `form:"warehouse_id"`
	Status      string `form:"status"`
	Page        int    `form:"page,default=1" binding:"min=1"`
	PageSize    int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type RecordActualReq struct {
	DetailID  int64 `json:"detail_id,string" binding:"required"`
	ActualQty int   `json:"actual_qty" binding:"min=0"`
}
