// Package repository reads dashboard aggregates without changing business records.
package repository

import (
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/dashboard/dto"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	taskmodel "gowms/internal/modules/task/model"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func warehouse(q *gorm.DB, id int64) *gorm.DB {
	if id > 0 {
		return q.Where("warehouse_id = ?", id)
	}
	return q
}

// Overview uses model-backed queries so both tenant callbacks (including exact
// tenant zero) and soft deletion apply to every aggregate and limited detail row.
// A short read-only repeatable-read transaction keeps the sections on one snapshot.
func (r *Repository) Overview(ctx context.Context, out *dto.Overview) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if out.Access.Basic {
			out.Warehouses = []dto.Warehouse{}
			if err := tx.Model(&basicmodel.Warehouse{}).Select("id, code, name").Order("code, id").Find(&out.Warehouses).Error; err != nil {
				return err
			}
			out.Locations = &dto.Locations{}
			if err := warehouse(tx.Model(&basicmodel.Location{}), out.WarehouseID).Where("status IN ?", []int{1, 2}).
				Select("COUNT(*) AS enabled, COALESCE(SUM(CASE WHEN status = 2 THEN 1 ELSE 0 END), 0) AS occupied").Scan(out.Locations).Error; err != nil {
				return err
			}
		}
		if out.Access.Inbound {
			out.Inbound = &dto.Orders{}
			if err := orderOverview(tx, &inboundmodel.ReceiptOrder{}, out, out.Inbound); err != nil {
				return err
			}
		}
		if out.Access.Outbound {
			out.Outbound = &dto.Orders{}
			if err := orderOverview(tx, &outboundmodel.ShipmentOrder{}, out, out.Outbound); err != nil {
				return err
			}
		}
		if out.Access.Tasks {
			q := warehouse(tx.Model(&taskmodel.Task{}), out.WarehouseID).Where("status IN ?", []string{"CREATED", "IN_PROGRESS"})
			if err := q.Select("task_type, status, COUNT(*) AS total, SUM(CASE WHEN created_at < ? THEN 1 ELSE 0 END) AS older_24h", out.GeneratedAt.Add(-24*time.Hour).Format(time.DateTime)).
				Group("task_type, status").Scan(&out.TaskCounts).Error; err != nil {
				return err
			}
			// Never select lease credentials or other internal task fields for the dashboard.
			if err := warehouse(tx.Model(&taskmodel.Task{}), out.WarehouseID).Where("status IN ?", []string{"CREATED", "IN_PROGRESS"}).
				Select("id, order_id, order_no, task_no, task_type, status, warehouse_id, created_at").Order("created_at, id").Limit(8).Find(&out.OldestTasks).Error; err != nil {
				return err
			}
		}
		if out.Access.Inventory {
			out.Inventory = &dto.Inventory{}
			if err := warehouse(tx.Model(&invmodel.Inventory{}), out.WarehouseID).Where("stock_quantity > 0").
				Select("COUNT(DISTINCT sku_id) AS skus, COUNT(*) AS `rows`, COALESCE(SUM(CASE WHEN allocated_quantity > 0 THEN 1 ELSE 0 END), 0) AS allocated_rows").Scan(out.Inventory).Error; err != nil {
				return err
			}
		}
		if out.Access.Stocktake {
			var total int64
			if err := warehouse(tx.Model(&stocktakemodel.StocktakeOrder{}), out.WarehouseID).Where("status = ?", "DRAFT").Count(&total).Error; err != nil {
				return err
			}
			out.DraftStocktakes = &total
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
}

func orderOverview(tx *gorm.DB, model any, scope *dto.Overview, out *dto.Orders) error {
	out.Statuses, out.Days, out.Recent = []dto.StatusCount{}, []dto.DayCount{}, []dto.Order{}
	if err := warehouse(tx.Model(model), scope.WarehouseID).
		Select("status, COUNT(*) AS total, SUM(CASE WHEN created_at >= ? AND created_at <= ? THEN 1 ELSE 0 END) AS today", scope.TodayFrom, scope.TodayTo).
		Group("status").Scan(&out.Statuses).Error; err != nil {
		return err
	}
	if err := warehouse(tx.Model(model), scope.WarehouseID).Where("created_at >= ? AND created_at <= ?", scope.TrendFrom, scope.TodayTo).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS date, COUNT(*) AS total").Group("DATE_FORMAT(created_at, '%Y-%m-%d')").Order("date").Scan(&out.Days).Error; err != nil {
		return err
	}
	return warehouse(tx.Model(model), scope.WarehouseID).Select("id, order_no, warehouse_id, status, created_at").Order("created_at DESC, id DESC").Limit(5).Find(&out.Recent).Error
}
