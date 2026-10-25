package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/dbutil"
	"gowms/internal/pkg/tenant"
)

func (r *Repository) SaveException(db *gorm.DB, row *model.WorkException) error {
	return db.Create(row).Error
}
func (r *Repository) Exception(db *gorm.DB, id int64) (*model.WorkException, error) {
	var row model.WorkException
	e := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error
	return &row, e
}
func (r *Repository) ListExceptions(db *gorm.DB, orderID int64, keyword string, page, size int) ([]*model.WorkException, int64, error) {
	q := db.Model(&model.WorkException{})
	if keyword != "" {
		q = q.Where("order_no LIKE ?", "%"+dbutil.LikePattern(keyword)+"%")
	}
	if orderID > 0 {
		q = q.Where("order_id=?", orderID)
	}
	var count int64
	if e := q.Count(&count).Error; e != nil {
		return nil, 0, e
	}
	rows := make([]*model.WorkException, 0)
	e := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, count, e
}
func (r *Repository) ResolveException(db *gorm.DB, id int64, close bool, reason, operator string) error {
	status := "RESOLVED"
	if close {
		status = "CLOSED"
	}
	return db.Model(&model.WorkException{}).Where("id=? AND status='OPEN'", id).Updates(map[string]any{"status": status, "resolution": reason, "resolved_by": operator, "resolved_at": time.Now()}).Error
}

// StopRemaining preserves quantities already shipped, and historical allocation/task
// records; cancelled rows retain unshipped partial picking for audit only.
func (r *Repository) StopRemaining(db *gorm.DB, order *model.ShipmentOrder, allocations []*model.Allocation) error {
	if _, e := r.CancelAllocationsByOrder(db, order.ID); e != nil {
		return e
	}
	if e := db.Model(&model.ShipmentOrderDetail{}).Where("order_id=?", order.ID).Updates(map[string]any{"allocated_qty": 0, "picked_qty": 0}).Error; e != nil {
		return e
	}
	for _, a := range allocations {
		if a.Status != model.AllocPicked {
			continue
		}
		if e := db.Model(&model.ShipmentOrderDetail{}).Where("id=? AND order_id=?", a.DetailID, order.ID).Updates(map[string]any{"allocated_qty": gorm.Expr("allocated_qty+?", a.PickedQty), "picked_qty": gorm.Expr("picked_qty+?", a.PickedQty)}).Error; e != nil {
			return e
		}
	}
	order.AllocatedQty = order.PickedQty
	order.Status = model.OrderException
	n, e := r.UpdateOrderProgress(db, order)
	if e != nil {
		return e
	}
	if n != 1 {
		return gorm.ErrInvalidData
	}
	return nil
}
func (r *Repository) Returnable(db *gorm.DB, orderID int64) ([]*dto.ReturnableRow, error) {
	tid := tenant.FromContext(db.Statement.Context)
	rows := make([]*dto.ReturnableRow, 0)
	e := db.Table("wms_allocation a").Joins("JOIN wms_shipment_order o ON o.id=a.order_id AND o.tenant_id=a.tenant_id AND o.deleted_at IS NULL").
		Joins("JOIN wms_shipment_order_detail d ON d.id=a.detail_id AND d.order_id=a.order_id AND d.tenant_id=a.tenant_id AND d.deleted_at IS NULL").
		Joins("LEFT JOIN (SELECT allocation_id,tenant_id,SUM(good_qty+bad_qty) AS returned FROM wms_customer_return WHERE tenant_id=? AND order_id=? GROUP BY allocation_id,tenant_id) r ON r.allocation_id=a.id AND r.tenant_id=a.tenant_id", tid, orderID).
		Where("a.tenant_id=? AND a.deleted_at IS NULL AND a.order_id=? AND a.status=? AND a.picked_qty>0 AND o.status IN ?", tid, orderID, model.AllocPicked, []model.OrderStatus{model.OrderShipped, model.OrderClosed}).
		Select("a.id AS allocation_id,a.sku_id,d.sku_code,d.sku_name,a.batch_no,a.picked_qty AS shipped_qty,COALESCE(r.returned,0) AS returned_qty,a.picked_qty-COALESCE(r.returned,0) AS returnable_qty").Order("a.id").Scan(&rows).Error
	return rows, e
}
func (r *Repository) ReturnedQty(db *gorm.DB, id int64) (int64, error) {
	var n int64
	e := db.Model(&model.CustomerReturn{}).Where("allocation_id=?", id).Select("COALESCE(SUM(good_qty+bad_qty),0)").Scan(&n).Error
	return n, e
}
func (r *Repository) SaveReturn(db *gorm.DB, row *model.CustomerReturn) error {
	return db.Create(row).Error
}
func (r *Repository) ListReturns(db *gorm.DB, orderID int64, keyword string, page, size int) ([]*model.CustomerReturn, int64, error) {
	q := db.Model(&model.CustomerReturn{})
	if keyword != "" {
		key := "%" + dbutil.LikePattern(keyword) + "%"
		q = q.Where("order_no LIKE ? OR return_no LIKE ?", key, key)
	}
	if orderID > 0 {
		q = q.Where("order_id=?", orderID)
	}
	var count int64
	if e := q.Count(&count).Error; e != nil {
		return nil, 0, e
	}
	rows := make([]*model.CustomerReturn, 0)
	e := q.Order("id DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error
	return rows, count, e
}

// ReturnLocations are available to return operators without granting catalog mutation
// permission. This is an advisory read; acceptance revalidates under locks.
func (r *Repository) ReturnLocations(db *gorm.DB, warehouseID int64, q *dto.ReturnLocationQuery) ([]*dto.ReturnLocation, int64, error) {
	query := db.Model(&basicmodel.Location{}).Where("warehouse_id=? AND status<>?", warehouseID, basicmodel.LocationStatusDisabled)
	var total int64
	if e := query.Count(&total).Error; e != nil {
		return nil, 0, e
	}
	rows := make([]*dto.ReturnLocation, 0)
	e := query.Select("id,code").Order("id").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&rows).Error
	return rows, total, e
}
func (r *Repository) AbortWave(db *gorm.DB, id int64, reason, operator string) error {
	if e := db.Model(&model.WaveOrder{}).Where("wave_id=? AND shipped_at IS NULL", id).Update("active_order_id", nil).Error; e != nil {
		return e
	}
	return db.Model(&model.Wave{}).Where("id=?", id).Updates(map[string]any{"status": model.WaveAborted, "abort_reason": reason, "aborted_by": operator, "aborted_at": time.Now(), "claimed_by": "", "lease_expire_at": nil}).Error
}
