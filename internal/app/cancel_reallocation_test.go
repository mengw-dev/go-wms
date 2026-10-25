package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	invdto "gowms/internal/modules/inventory/dto"
	invmodel "gowms/internal/modules/inventory/model"
	outdto "gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/tx"
)

func TestCancelUsesCurrentAllocationsAfterExceptionReallocation(t *testing.T) {
	s, inventory := newClosureStack(t)
	order, task := s.newPickOrder(t, "CANCEL-SNAPSHOT", 5)
	source := s.inventory(t)
	if err := tx.New(s.db).Tx(s.ctx, func(db *gorm.DB) error {
		return inventory.Increase(s.ctx, db, &invapi.IncreaseReq{WarehouseID: s.warehouseID, LocationID: s.locationID, SKUID: s.skuID, BatchNo: "REPLACEMENT", Quantity: 20})
	}); err != nil {
		t.Fatal(err)
	}
	var target invmodel.Inventory
	if err := s.db.WithContext(s.ctx).Where("batch_no=?", "REPLACEMENT").First(&target).Error; err != nil {
		t.Fatal(err)
	}
	type cancelContextKey struct{}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	arrived, resume := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(resume) }) }
	defer release()
	const callback = "test:cancel_before_task_lock"
	if err := s.db.Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
		if q.Statement.Table != "wms_shipment_order" || q.Statement.Context.Value(cancelContextKey{}) != true || q.Error != nil {
			return
		}
		once.Do(func() {
			close(arrived)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.db.Callback().Query().Remove(callback) }()
	done := make(chan error, 1)
	received := false
	defer func() {
		release()
		cancel()
		if !received {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("cancel worker did not stop")
			}
		}
	}()
	go func() { done <- s.out.Cancel(context.WithValue(ctx, cancelContextKey{}, true), order.ID, "manager") }()
	select {
	case <-arrived:
	case <-ctx.Done():
		t.Fatal("cancel did not establish its initial read")
	}
	ack, err := s.out.ReportException(s.ctx, &outdto.ExceptionRequest{TaskID: task.ID, Kind: "DAMAGE", Quantity: 1, ConfirmReturned: true, Reason: "returned"}, "manager", "exception")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = inventory.FreezeStock(s.ctx, &invdto.FreezeRequest{InventoryID: source.ID, Quantity: 19, Reason: "quarantine original batch"}, "freeze", "manager"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.out.ResolveWorkException(s.ctx, ack.ObjectID, &outdto.ResolveExceptionRequest{Reason: "use replacement batch"}, "manager", "resolve"); err != nil {
		t.Fatal(err)
	}
	if _, err = inventory.FreezeStock(s.ctx, &invdto.FreezeRequest{InventoryID: source.ID, Quantity: 5, Unfreeze: true, Reason: "release for another order"}, "unfreeze", "manager"); err != nil {
		t.Fatal(err)
	}
	other, _ := s.newPickOrder(t, "OTHER-RESERVATION", 5)
	release()
	err = <-done
	received = true
	if err != nil {
		t.Fatal(err)
	}
	if err = s.db.WithContext(s.ctx).First(source, source.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err = s.db.WithContext(s.ctx).First(&target, target.ID).Error; err != nil {
		t.Fatal(err)
	}
	if source.AllocatedQty != 5 || source.AvailableQty != 0 || target.AllocatedQty != 0 || target.AvailableQty != 20 {
		t.Fatalf("cancel released another order's batch: source=%+v target=%+v", source, target)
	}
	if s.reloadOrder(t, other.ID).AllocatedQty != 5 {
		t.Fatal("other order reservation changed")
	}
}
