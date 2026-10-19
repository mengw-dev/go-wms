package bootstrap

import (
	"fmt"

	"gorm.io/gorm"

	"gowms/internal/pkg/quantity"
)

// auditQuantityColumns 在开发 AutoMigrate 可能将历史 BIGINT 收窄为 INT 前检查实际数据。
// 只在应用启动、尚未接收写入时运行；生产始终使用版本化 SQL，不运行 AutoMigrate。
func auditQuantityColumns(db *gorm.DB) error {
	columns := map[string][]string{
		"wms_inventory":             {"stock_quantity", "available_quantity", "allocated_quantity"},
		"wms_inventory_trans":       {"quantity_change", "before_quantity", "after_quantity", "available_before", "available_after"},
		"wms_receipt_order":         {"expected_qty", "received_qty", "defective_qty"},
		"wms_receipt_order_detail":  {"expected_qty", "received_qty", "defective_qty"},
		"wms_shipment_order":        {"expected_qty", "allocated_qty", "picked_qty"},
		"wms_shipment_order_detail": {"expected_qty", "allocated_qty", "picked_qty"},
		"wms_allocation":            {"allocated_qty", "picked_qty"},
		"wms_task":                  {"target_qty", "done_qty"},
		"wms_stocktake_detail":      {"book_qty", "actual_qty", "diff_qty"},
	}
	for table, names := range columns {
		if !db.Migrator().HasTable(table) {
			continue
		}
		for _, column := range names {
			if !db.Migrator().HasColumn(table, column) {
				continue
			}
			var invalid int
			// 标识符全部来自上方固定表，不拼接用户输入；包含软删数据和全部租户。
			query := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s` WHERE `%s` > ? OR `%s` < ?)", table, column, column)
			if err := db.Raw(query, quantity.Max, -quantity.Max-1).Scan(&invalid).Error; err != nil { // 启动审计覆盖全部 tenant_id，不按请求租户过滤。
				return err
			}
			if invalid != 0 {
				return fmt.Errorf("quantity audit failed for %s.%s: values outside signed INT range; migration stopped without changing data", table, column)
			}
		}
	}
	return nil
}
