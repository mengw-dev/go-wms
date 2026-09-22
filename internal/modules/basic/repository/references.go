package repository

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/pkg/tenant"
)

// CountWarehouseReferences 统计仓库下的业务引用。库存单独通过 StockChecker
// 查询，便于返回更明确的错误；这里覆盖库位、任务和各类单据。
func (r *Repository) CountWarehouseReferences(ctx context.Context, db *gorm.DB, warehouseID int64) (int64, error) {
	return r.countReferences(ctx, db, warehouseID, []referenceColumn{
		{"wms_location", "warehouse_id"},
		{"wms_task", "warehouse_id"},
		{"wms_receipt_order", "warehouse_id"},
		{"wms_shipment_order", "warehouse_id"},
		{"wms_stocktake_order", "warehouse_id"},
	})
}

// CountLocationReferences 统计库位上的任务和单据引用。
func (r *Repository) CountLocationReferences(ctx context.Context, db *gorm.DB, locationID int64) (int64, error) {
	return r.countReferences(ctx, db, locationID, []referenceColumn{
		{"wms_task", "location_id"},
		{"wms_allocation", "location_id"},
		{"wms_stocktake_order", "location_id"},
		{"wms_stocktake_detail", "location_id"},
	})
}

// CountSKUReferences 统计货品上的任务、单据明细和分配引用。
func (r *Repository) CountSKUReferences(ctx context.Context, db *gorm.DB, skuID int64) (int64, error) {
	return r.countReferences(ctx, db, skuID, []referenceColumn{
		{"wms_task", "sku_id"},
		{"wms_receipt_order_detail", "sku_id"},
		{"wms_shipment_order_detail", "sku_id"},
		{"wms_allocation", "sku_id"},
		{"wms_stocktake_detail", "sku_id"},
	})
}

type referenceColumn struct {
	table  string
	column string
}

func (r *Repository) countReferences(ctx context.Context, db *gorm.DB, id int64, refs []referenceColumn) (int64, error) {
	for _, ref := range refs {
		// Unscoped 保留历史引用：即使业务单据已经软删除，也不能让基础资料被硬删后留下孤儿行。
		q := db.WithContext(ctx).Unscoped().Table(ref.table).Where(ref.column+" = ?", id)
		if tenantID, scoped := tenant.Scope(ctx); scoped {
			q = q.Where("tenant_id = ?", tenantID)
		}
		var count int64
		if err := q.Count(&count).Error; err != nil {
			return 0, err
		}
		if count > 0 {
			return count, nil
		}
	}
	return 0, nil
}
