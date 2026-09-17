package repository

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/dbutil"
)

type Repository struct{}

func New() *Repository { return &Repository{} }

// ---------- 出库单 ----------

func (r *Repository) CreateOrder(tx *gorm.DB, order *model.ShipmentOrder, details []*model.ShipmentOrderDetail) error {
	if err := tx.Create(order).Error; err != nil {
		return err
	}
	for _, d := range details {
		d.OrderID = order.ID
	}
	return tx.CreateInBatches(details, 100).Error
}

func (r *Repository) GetOrderForUpdate(tx *gorm.DB, id int64) (*model.ShipmentOrder, error) {
	var o model.ShipmentOrder
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&o, id).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) GetOrder(ctx context.Context, db *gorm.DB, id int64) (*model.ShipmentOrder, error) {
	var o model.ShipmentOrder
	if err := db.WithContext(ctx).First(&o, id).Error; err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) GetOrderByBizNo(ctx context.Context, db *gorm.DB, bizNo string) (*model.ShipmentOrder, error) {
	var o model.ShipmentOrder
	err := db.WithContext(ctx).Where("biz_order_no = ?", bizNo).First(&o).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// UpdateStatus 状态推进（乐观：WHERE status = from）。
func (r *Repository) UpdateStatus(tx *gorm.DB, id int64, from, to model.OrderStatus) (int64, error) {
	res := tx.Model(&model.ShipmentOrder{}).Where("id = ? AND status = ?", id, from).Update("status", to)
	return res.RowsAffected, res.Error
}

func (r *Repository) DeleteOrder(tx *gorm.DB, id int64) error {
	if err := tx.Delete(&model.ShipmentOrder{}, id).Error; err != nil {
		return err
	}
	return tx.Where("order_id = ?", id).Delete(&model.ShipmentOrderDetail{}).Error
}

// ---------- 明细 ----------

func (r *Repository) ListDetails(tx *gorm.DB, orderID int64) ([]*model.ShipmentOrderDetail, error) {
	var list []*model.ShipmentOrderDetail
	err := tx.Where("order_id = ?", orderID).Order("id").Find(&list).Error
	return list, err
}

// GetDetailForUpdate 锁定明细行。
func (r *Repository) GetDetailForUpdate(tx *gorm.DB, detailID int64) (*model.ShipmentOrderDetail, error) {
	var d model.ShipmentOrderDetail
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&d, detailID).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// UpdateDetailAllocated 累加明细分配量。
func (r *Repository) UpdateDetailAllocated(tx *gorm.DB, d *model.ShipmentOrderDetail) error {
	return tx.Model(&model.ShipmentOrderDetail{}).Where("id = ?", d.ID).
		Update("allocated_qty", d.AllocatedQty).Error
}

// IncrDetailPicked 原子累加明细拣货量（picked_qty = picked_qty + delta）。
func (r *Repository) IncrDetailPicked(tx *gorm.DB, detailID int64, delta int) error {
	return tx.Model(&model.ShipmentOrderDetail{}).Where("id = ?", detailID).
		Update("picked_qty", gorm.Expr("picked_qty + ?", delta)).Error
}

// UpdateOrderProgress 审核分配后一次性写入主单分配量并推进状态（version 乐观锁）。
// 返回 RowsAffected：0 表示版本冲突（已被并发事务修改），调用方应返回 VersionBad 或重试。
func (r *Repository) UpdateOrderProgress(tx *gorm.DB, o *model.ShipmentOrder) (int64, error) {
	res := tx.Model(&model.ShipmentOrder{}).Where("id = ? AND version = ?", o.ID, o.Version).Updates(map[string]any{
		"allocated_qty": o.AllocatedQty, "picked_qty": o.PickedQty,
		"status": o.Status, "version": gorm.Expr("version + 1"),
	})
	return res.RowsAffected, res.Error
}

// IncrOrderPicked 原子累加主单拣货量（picked_qty = picked_qty + delta，version +1）。
// 拣货并发时不再依赖内存读-改-写；返回 RowsAffected，0 表示版本冲突需重试。
func (r *Repository) IncrOrderPicked(tx *gorm.DB, id int64, version int, delta int) (int64, error) {
	res := tx.Model(&model.ShipmentOrder{}).Where("id = ? AND version = ?", id, version).
		Updates(map[string]any{
			"picked_qty": gorm.Expr("picked_qty + ?", delta),
			"version":    gorm.Expr("version + 1"),
		})
	return res.RowsAffected, res.Error
}

func (r *Repository) ListOrders(ctx context.Context, db *gorm.DB, warehouseID int64, status, keyword, createdAtFrom, createdAtTo string, page, size int) ([]*model.ShipmentOrder, int64, error) {
	q := db.WithContext(ctx).Model(&model.ShipmentOrder{})
	if warehouseID > 0 {
		q = q.Where("warehouse_id = ?", warehouseID)
	}
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if keyword != "" {
		q = q.Where("order_no LIKE ? OR biz_order_no LIKE ?", "%"+dbutil.LikePattern(keyword)+"%", "%"+dbutil.LikePattern(keyword)+"%")
	}
	if createdAtFrom != "" {
		q = q.Where("created_at >= ?", createdAtFrom)
	}
	if createdAtTo != "" {
		q = q.Where("created_at <= ?", createdAtTo)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []*model.ShipmentOrder
	err := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&list).Error
	return list, total, err
}

// ---------- 分配行 ----------

func (r *Repository) CreateAllocations(tx *gorm.DB, list []*model.Allocation) error {
	return tx.CreateInBatches(list, 100).Error
}

func (r *Repository) ListAllocations(tx *gorm.DB, orderID int64) ([]*model.Allocation, error) {
	var list []*model.Allocation
	err := tx.Where("order_id = ?", orderID).Order("id").Find(&list).Error
	return list, err
}

func (r *Repository) GetAllocationForUpdate(tx *gorm.DB, id int64) (*model.Allocation, error) {
	var a model.Allocation
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&a, id).Error
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// IncrAllocationPicked 原子累加分配行拣货量，拣满时置为 PICKED（version 乐观锁 + status 条件）。
// delta 为本次拣货数，toStatus 传入拣满后的 AllocPicked 或未拣满时的 AllocAllocated。
// 返回 RowsAffected：0 表示分配行已被并发改动（状态非 ALLOCATED 或版本冲突），需重试。
func (r *Repository) IncrAllocationPicked(tx *gorm.DB, a *model.Allocation, delta int, toStatus model.AllocationStatus) (int64, error) {
	res := tx.Model(&model.Allocation{}).
		Where("id = ? AND version = ? AND status = ?", a.ID, a.Version, model.AllocAllocated).
		Updates(map[string]any{
			"picked_qty": gorm.Expr("picked_qty + ?", delta),
			"status":     toStatus,
			"version":    gorm.Expr("version + 1"),
		})
	return res.RowsAffected, res.Error
}

// CancelAllocationsByOrder 取消单据全部已分配未拣货行。
func (r *Repository) CancelAllocationsByOrder(tx *gorm.DB, orderID int64) (int64, error) {
	res := tx.Model(&model.Allocation{}).
		Where("order_id = ? AND status = ?", orderID, model.AllocAllocated).
		Update("status", model.AllocCancelled)
	return res.RowsAffected, res.Error
}
