package bootstrap

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/tenant"
)

// ResetDemoData 硬删除指定租户的演示业务数据并重新写入默认演示数据。
// 用户、角色、迁移记录和操作日志不会删除。
// 租户 ID 经 ctx 传播：删除和种子都由 GORM 租户回调自动限定在该租户内，
// 各演示账号互不影响；tenantID <= 0（默认租户/平台旁路）时保持全局重置的旧行为。
func ResetDemoData(ctx context.Context, db *gorm.DB, tenantID int64) error {
	ctx = tenant.WithTenant(ctx, tenantID)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		models := []any{
			&taskmodel.Task{},
			&outboundmodel.Allocation{},
			&outboundmodel.ShipmentOrderDetail{},
			&outboundmodel.ShipmentOrder{},
			&inboundmodel.ReceiptOrderDetail{},
			&inboundmodel.ReceiptOrder{},
			&inboundmodel.ImportTask{},
			&stocktakemodel.StocktakeDetail{},
			&stocktakemodel.StocktakeOrder{},
			&invmodel.InventoryTrans{},
			&invmodel.Inventory{},
			&model.Location{},
			&model.SKU{},
			&model.Warehouse{},
		}
		for _, item := range models {
			if err := tx.Unscoped().Where("1 = 1").Delete(item).Error; err != nil {
				return err
			}
		}
		return seedDemoData(tx)
	})
}
