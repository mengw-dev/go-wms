// Package model 定义仓库、库位和 SKU 的持久化结构及编码唯一约束。
package model

import "gowms/internal/modules/system/model"

// Warehouse 仓库是库存和多数业务单据的租户内范围边界。
type Warehouse struct {
	model.Base
	TenantID int64  `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_warehouse_code,priority:1;index:idx_wh_tenant"`
	Code     string `json:"code" gorm:"size:32;uniqueIndex:uk_warehouse_code,priority:2;not null"`
	Name     string `json:"name" gorm:"size:64;not null"`
	Remark   string `json:"remark" gorm:"size:255"`
	Status   int    `json:"status" gorm:"default:1"` // 1 启用 0 禁用
}

func (Warehouse) TableName() string { return "wms_warehouse" }

// Location 库位属于一个仓库，是库存四元组中的实际存放位置。
type Location struct {
	model.Base
	TenantID    int64  `json:"tenant_id,string" gorm:"not null;default:0;index:idx_loc_tenant;uniqueIndex:uk_loc_wh_code,priority:1"`
	WarehouseID int64  `json:"warehouse_id,string" gorm:"not null;uniqueIndex:uk_loc_wh_code,priority:2"`
	Code        string `json:"code" gorm:"size:64;not null;uniqueIndex:uk_loc_wh_code,priority:3"` // 格式 {库区}-{排}-{列}，如 A01-02-03
	Zone        string `json:"zone" gorm:"size:32"`                                                // 库区，取编码第一段
	Status      int    `json:"status" gorm:"default:1"`                                            // 1 空闲 2 占用 0 禁用
}

func (Location) TableName() string { return "wms_location" }

// 库位状态
const (
	LocationStatusDisabled = 0
	LocationStatusIdle     = 1
	LocationStatusOccupied = 2
)

// SKU 货品在租户内以编码和条码分别唯一。
type SKU struct {
	model.Base
	TenantID int64  `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_sku_code,priority:1;uniqueIndex:uk_sku_barcode,priority:1"`
	Code     string `json:"code" gorm:"size:64;uniqueIndex:uk_sku_code,priority:2;not null"`
	Barcode  string `json:"barcode" gorm:"size:64;uniqueIndex:uk_sku_barcode,priority:2;not null"`
	Name     string `json:"name" gorm:"size:128;not null"`
	Spec     string `json:"spec" gorm:"size:128"`
	Unit     string `json:"unit" gorm:"size:16"`
	Status   int    `json:"status" gorm:"default:1"`
}

func (SKU) TableName() string { return "wms_sku" }
