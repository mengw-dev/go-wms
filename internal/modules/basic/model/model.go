package model

import "gowms/internal/modules/system/model"

type Warehouse struct {
	model.Base
	Code   string `json:"code" gorm:"size:32;uniqueIndex;not null"`
	Name   string `json:"name" gorm:"size:64;not null"`
	Remark string `json:"remark" gorm:"size:255"`
	Status int    `json:"status" gorm:"default:1"` // 1 启用 0 禁用
}

func (Warehouse) TableName() string { return "wms_warehouse" }

type Location struct {
	model.Base
	WarehouseID int64  `json:"warehouse_id" gorm:"not null;uniqueIndex:uk_location_warehouse_code,priority:1"`
	Code        string `json:"code" gorm:"size:64;not null;uniqueIndex:uk_location_warehouse_code,priority:2"` // 格式 {库区}-{排}-{列}，如 A01-02-03
	Zone        string `json:"zone" gorm:"size:32"`                                                            // 库区，取编码第一段
	Status      int    `json:"status" gorm:"default:1"`                                                        // 1 空闲 2 占用 0 禁用
}

func (Location) TableName() string { return "wms_location" }

// 库位状态
const (
	LocationStatusDisabled = 0
	LocationStatusIdle     = 1
	LocationStatusOccupied = 2
)

type SKU struct {
	model.Base
	Code    string `json:"code" gorm:"size:64;uniqueIndex;not null"`
	Barcode string `json:"barcode" gorm:"size:64;uniqueIndex;not null"`
	Name    string `json:"name" gorm:"size:128;not null"`
	Spec    string `json:"spec" gorm:"size:128"`
	Unit    string `json:"unit" gorm:"size:16"`
	Status  int    `json:"status" gorm:"default:1"`
}

func (SKU) TableName() string { return "wms_sku" }
