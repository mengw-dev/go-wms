package dto

import (
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/typex"
)

type WaveQuery struct {
	WarehouseID int64  `form:"warehouse_id" binding:"gte=0"`
	Keyword     string `form:"keyword"`
	Page        int    `form:"page,default=1" binding:"min=1"`
	PageSize    int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

// WaveCandidateQuery previews automatic selection; confirmation sends exact order IDs.
type WaveCandidateQuery struct {
	WarehouseID int64  `form:"warehouse_id" binding:"required,gt=0"`
	Keyword     string `form:"keyword"`
	Limit       int    `form:"limit,default=20" binding:"min=1,max=50"`
}

type CreateWaveReq struct {
	OrderIDs typex.Int64List `json:"order_ids" binding:"required,min=1,max=50"`
	Remark   string          `json:"remark" binding:"max=255"`
}

type WaveQuantityReq struct {
	Qty          int    `json:"qty" binding:"required,min=1,max=2147483647"`
	SKUCode      string `json:"sku_code" binding:"required,max=64"`
	LocationCode string `json:"location_code" binding:"max=64"`
	BatchNo      string `json:"batch_no" binding:"max=64"`
	SlotCode     string `json:"slot_code" binding:"max=16"`
}

type WaveDetailResp struct {
	Wave   *model.Wave        `json:"wave"`
	Orders []*model.WaveOrder `json:"orders"`
	Lines  []*model.WaveLine  `json:"lines"`
	Items  []*model.WaveItem  `json:"items"`
}

type WaveAck struct {
	WaveID int64 `json:"wave_id,string"`
}
