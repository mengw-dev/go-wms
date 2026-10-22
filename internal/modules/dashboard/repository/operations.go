package repository

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/dashboard/dto"
	inboundmodel "gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/tenant"
)

func activityOverview(ctx context.Context, tx *gorm.DB, out *dto.Overview) error {
	// Ledger has no soft-delete column. Its inventory join is tenant-matched and
	// excludes deleted rows; raw tables must not rely on model tenant callbacks.
	query := func() *gorm.DB {
		q := tx.Table("wms_inventory_trans t").
			Joins("JOIN wms_inventory i ON i.id=t.inventory_id AND i.tenant_id=t.tenant_id AND i.deleted_at IS NULL").
			Where("t.trans_type IN ? AND t.created_at >= ? AND t.created_at <= ?", []string{"RECEIVE", "SHIP"}, out.TodayFrom, out.TodayTo)
		if id, scoped := tenant.Scope(ctx); scoped {
			q = q.Where("t.tenant_id = ?", id)
		}
		if out.WarehouseID > 0 {
			q = q.Where("i.warehouse_id = ?", out.WarehouseID)
		}
		return q
	}
	out.Activity = &dto.Activity{Recent: []dto.ActivityRecord{}}
	if err := query().Select("COALESCE(SUM(t.trans_type='RECEIVE'),0) AS putaway_records, COALESCE(SUM(t.trans_type='SHIP'),0) AS shipment_records").Scan(out.Activity).Error; err != nil {
		return err
	}
	return query().Select("t.id,t.inventory_id,t.trans_type,t.order_no,t.task_no,t.created_at").
		Order("t.created_at DESC,t.id DESC").Limit(8).Scan(&out.Activity.Recent).Error
}

func importOverview(tx *gorm.DB, out *dto.Overview) error {
	// Do not use ListImportTasks: its batch selector deliberately hides files
	// without surviving orders, including failures before the first row import.
	query := func() *gorm.DB {
		return tx.Model(&inboundmodel.ImportTask{}).
			Where("created_at >= ? AND created_at <= ?", out.TrendFrom, out.TodayTo).
			Where("status = ? OR (status = ? AND fail_rows > 0)", "FAILED", "COMPLETED")
	}
	out.ImportIssues = &dto.Imports{Recent: []dto.ImportIssue{}}
	if err := query().Count(&out.ImportIssues.Total).Error; err != nil {
		return err
	}
	return query().Select("task_id,file_name,status,success_rows,fail_rows,error_msg,created_at").
		Order("created_at DESC,id DESC").Limit(5).Find(&out.ImportIssues.Recent).Error
}
