package dto

type FreezeRequest struct {
	InventoryID int64  `json:"inventory_id,string" binding:"required,gt=0"`
	Quantity    int    `json:"quantity" binding:"required,gt=0,lte=2147483647"`
	Unfreeze    bool   `json:"unfreeze"`
	Reason      string `json:"reason" binding:"required,max=255"`
}
