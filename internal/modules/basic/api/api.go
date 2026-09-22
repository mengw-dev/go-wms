// Package api 定义 basic 模块提供给其他业务模块的仓库、库位和 SKU 能力。
package api

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
)

// StockChecker 库存存在性校验，由 inventory 模块实现（app 组装注入）。
type StockChecker interface {
	HasStockByWarehouse(ctx context.Context, db *gorm.DB, warehouseID int64) (bool, error)
	HasStockByLocation(ctx context.Context, db *gorm.DB, locationID int64) (bool, error)
	HasStockBySKU(ctx context.Context, db *gorm.DB, skuID int64) (bool, error)
}

// BasicAPI basic 模块对外接口。
type BasicAPI interface {
	ValidateWarehouse(ctx context.Context, id int64) error                        // 存在且启用
	ValidateLocation(ctx context.Context, id int64) error                         // 存在且非禁用
	ValidateLocationInWarehouse(ctx context.Context, warehouseID, id int64) error // 存在、非禁用且属于指定仓库
	ValidateSKU(ctx context.Context, id int64) error                              // 存在且启用
	GetSKU(ctx context.Context, id int64) (*model.SKU, error)
	GetLocation(ctx context.Context, id int64) (*model.Location, error)
	GetWarehouseByCode(ctx context.Context, code string) (*model.Warehouse, error)
	GetSKUByCode(ctx context.Context, code string) (*model.SKU, error)
	UpdateLocationStatusInTx(ctx context.Context, tx *gorm.DB, id int64, status int) error
}
