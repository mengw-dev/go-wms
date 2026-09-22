package dto

import "time"

type WarehouseReq struct {
	Code   string `json:"code" binding:"required,max=32"`
	Name   string `json:"name" binding:"required,max=64"`
	Remark string `json:"remark" binding:"max=255"`
}

type StatusReq struct {
	Status *int `json:"status" binding:"required"`
}

type CommonQuery struct {
	Keyword  string `form:"keyword"`
	Page     int    `form:"page,default=1" binding:"min=1"`
	PageSize int    `form:"page_size,default=10" binding:"min=1,max=100"`
}

type WarehouseQuery struct {
	Status *int `form:"status" binding:"omitempty,oneof=0 1"`
	CommonQuery
}

type LocationQuery struct {
	Zone        string `form:"zone"`
	Status      *int   `form:"status" binding:"omitempty,oneof=0 1 2"`
	WarehouseID int64  `form:"warehouse_id"`
	CommonQuery
}

// LocationBatchReq 批量初始化库位：按 库区-排-列 规则批量生成，已存在编码跳过（幂等）。
type LocationBatchReq struct {
	WarehouseID int64  `json:"warehouse_id,string" binding:"required"`
	Zone        string `json:"zone" binding:"required,max=32"` // 库区，如 A01
	RowFrom     int    `json:"row_from" binding:"required,min=1"`
	RowTo       int    `json:"row_to" binding:"required,min=1"`
	ColFrom     int    `json:"col_from" binding:"required,min=1"`
	ColTo       int    `json:"col_to" binding:"required,min=1"`
}

type SKUReq struct {
	Code    string `json:"code" binding:"required,max=64"`
	Barcode string `json:"barcode" binding:"required,max=64"`
	Name    string `json:"name" binding:"required,max=128"`
	Spec    string `json:"spec" binding:"max=128"`
	Unit    string `json:"unit" binding:"max=16"`
}
type WarehouseResp struct {
	ID        int64     `json:"id,string"`
	TenantID  int64     `json:"tenant_id,string"`
	Code      string    `json:"code"`
	Name      string    `json:"name"`
	Remark    string    `json:"remark"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LocationResp struct {
	ID          int64     `json:"id,string"`
	TenantID    int64     `json:"tenant_id,string"`
	WarehouseID int64     `json:"warehouse_id,string"`
	Code        string    `json:"code"`
	Zone        string    `json:"zone"`
	Status      int       `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type SKUResp struct {
	ID        int64     `json:"id,string"`
	TenantID  int64     `json:"tenant_id,string"`
	Code      string    `json:"code"`
	Barcode   string    `json:"barcode"`
	Name      string    `json:"name"`
	Spec      string    `json:"spec"`
	Unit      string    `json:"unit"`
	Status    int       `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
