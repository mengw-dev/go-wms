package dto

type ExceptionRequest struct {
	ConfirmReturned bool   `json:"confirm_returned"`
	TaskID          int64  `json:"task_id,string" binding:"required,gt=0"`
	Kind            string `json:"kind" binding:"required,oneof=SHORTAGE DAMAGE"`
	Quantity        int    `json:"quantity" binding:"required,gt=0,lte=2147483647"`
	Reason          string `json:"reason" binding:"required,max=255"`
}
type ResolveExceptionRequest struct {
	Close  bool   `json:"close"`
	Reason string `json:"reason" binding:"required,max=255"`
}
type AbortWaveRequest struct {
	ConfirmReturned bool   `json:"confirm_returned"`
	Reason          string `json:"reason" binding:"required,max=255"`
}
type CustomerReturnRequest struct {
	AllocationID int64  `json:"allocation_id,string" binding:"required,gt=0"`
	LocationID   int64  `json:"location_id,string" binding:"required,gt=0"`
	GoodQty      int    `json:"good_qty" binding:"gte=0,lte=2147483647"`
	BadQty       int    `json:"bad_qty" binding:"gte=0,lte=2147483647"`
	Reason       string `json:"reason" binding:"required,max=255"`
}
type ClosureQuery struct {
	Keyword  string `form:"keyword" binding:"max=64"`
	OrderID  int64  `form:"order_id" binding:"gte=0"`
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type ReturnLocationQuery struct {
	Page     int `form:"page,default=1" binding:"min=1"`
	PageSize int `form:"page_size,default=20" binding:"min=1,max=100"`
}
type ReturnLocation struct {
	ID   int64  `json:"id,string"`
	Code string `json:"code"`
}
type ClosureAck struct {
	ObjectID int64 `json:"object_id,string"`
}

type ReturnableRow struct {
	AllocationID  int64  `json:"allocation_id,string"`
	SKUID         int64  `json:"sku_id,string" gorm:"column:sku_id"`
	SKUCode       string `json:"sku_code"`
	SKUName       string `json:"sku_name"`
	BatchNo       string `json:"batch_no"`
	ShippedQty    int    `json:"shipped_qty"`
	ReturnedQty   int64  `json:"returned_qty"`
	ReturnableQty int64  `json:"returnable_qty"`
}
