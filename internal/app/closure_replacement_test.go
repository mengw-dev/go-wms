package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	invmodel "gowms/internal/modules/inventory/model"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
)

func TestWaveWaitsForWarehouseBeforeTakingSKULocks(t *testing.T) {
	s := newWaveStack(t)
	o, _ := s.newPickOrder(t, "CATALOG-ORDER", 5)
	ctx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
	defer cancel()
	holder := s.db.WithContext(ctx).Begin()
	defer holder.Rollback()
	if _, err := basicrepo.New().GetWarehouseForUpdate(ctx, holder, s.warehouseID); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int64
	const callback = "test:wave_catalog_attempts"
	if err := s.db.Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
		if q.Statement.Table == "wms_task" {
			if _, exists := q.Statement.Clauses["FOR"]; exists {
				attempts.Add(1)
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.db.Callback().Query().Remove(callback) }()
	type result struct {
		ack *outdto.WaveAck
		err error
	}
	done := make(chan result, 1)
	received := false
	defer func() {
		cancel()
		_ = holder.Rollback().Error
		if !received {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("wave worker did not stop")
			}
		}
	}()
	go func() {
		ack, err := s.out.CreateWave(ctx, &outdto.CreateWaveReq{OrderIDs: []int64{o.ID}}, "picker", "catalog-wave")
		done <- result{ack, err}
	}()
	for {
		var waiting int64
		if err := s.db.WithContext(ctx).Raw("SELECT COUNT(*) FROM performance_schema.data_lock_waits w JOIN performance_schema.data_locks r ON w.REQUESTING_ENGINE_LOCK_ID=r.ENGINE_LOCK_ID WHERE r.OBJECT_SCHEMA=DATABASE() AND r.OBJECT_NAME='wms_warehouse'").Scan(&waiting).Error; err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		select {
		case r := <-done:
			received = true
			t.Fatalf("wave bypassed warehouse gate: %v", r.err)
		case <-ctx.Done():
			t.Fatal("warehouse wait not observed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	// The counterpart follows warehouse -> SKU, the order used by stock/reference writes.
	if err := holder.Model(&basicmodel.SKU{}).Where("id=?", s.skuID).Update("name", "catalog changed").Error; err != nil {
		t.Fatal(err)
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatal(err)
	}
	r := <-done
	received = true
	if r.err != nil || attempts.Load() != 1 {
		t.Fatalf("wave relied on deadlock retry: attempts=%d err=%v", attempts.Load(), r.err)
	}
	if line := waveDetail(t, s, r.ack.WaveID).Lines[0]; line.SKUName != "catalog changed" {
		t.Fatalf("stale SKU snapshot: %+v", line)
	}
}

func TestReplacementTasksSupportWaveLifecycleAndCancellation(t *testing.T) {
	for _, action := range []string{"ship", "cancel-wave", "abort-wave", "cancel-order"} {
		t.Run(action, func(t *testing.T) {
			s, _ := newClosureStack(t)
			order, original := s.newPickOrder(t, "REPLACEMENT", 5)
			if _, err := s.out.Pick(s.ctx, original.ID, 2, "picker", nil, "", "partial-before-return"); err != nil {
				t.Fatal(err)
			}
			ack, err := s.out.ReportException(s.ctx, &outdto.ExceptionRequest{TaskID: original.ID, Kind: "DAMAGE", Quantity: 1, Reason: "unshipped goods returned", ConfirmReturned: true}, "manager", "exception")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.out.ResolveWorkException(s.ctx, ack.ObjectID, &outdto.ResolveExceptionRequest{Reason: "allocate replacement"}, "manager", "resolve"); err != nil {
				t.Fatal(err)
			}
			if action != "cancel-order" {
				candidates, err := s.out.WaveCandidates(s.ctx, &outdto.WaveCandidateQuery{WarehouseID: s.warehouseID, Limit: 20})
				if err != nil || len(candidates) != 1 || candidates[0].ID != order.ID {
					t.Fatalf("replacement excluded by history: %+v %v", candidates, err)
				}
				wave, err := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{order.ID}}, "picker", "wave")
				if err != nil {
					t.Fatal(err)
				}
				d := waveDetail(t, s, wave.WaveID)
				if len(d.Items) != 1 || len(d.Lines) != 1 {
					t.Fatalf("cancelled attempts leaked into wave: %+v", d)
				}
				if _, err = s.out.ClaimWave(s.ctx, wave.WaveID, "picker", "claim"); err != nil {
					t.Fatal(err)
				}
				if action == "cancel-wave" {
					if _, err = s.out.CancelWave(s.ctx, wave.WaveID, "picker", "cancel-wave"); err != nil {
						t.Fatal(err)
					}
				} else {
					line := d.Lines[0]
					if _, err = s.out.CollectWave(s.ctx, wave.WaveID, line.ID, outdto.WaveQuantityReq{Qty: int(line.TargetQty), SKUCode: line.SKUCode, LocationCode: line.LocationCode, BatchNo: line.BatchNo}, "picker", "collect"); err != nil {
						t.Fatal(err)
					}
					if action == "abort-wave" {
						if _, err = s.out.AbortWorkWave(s.ctx, wave.WaveID, &outdto.AbortWaveRequest{ConfirmReturned: true, Reason: "returned from collection"}, "manager", "abort"); err != nil {
							t.Fatal(err)
						}
					} else {
						item := d.Items[0]
						if _, err = s.out.SortWave(s.ctx, wave.WaveID, item.ID, outdto.WaveQuantityReq{Qty: item.TargetQty, SKUCode: line.SKUCode, BatchNo: line.BatchNo, SlotCode: d.Orders[0].SlotCode}, "picker", "sort"); err != nil {
							t.Fatal(err)
						}
						if _, err = s.out.ShipWaveOrder(s.ctx, wave.WaveID, d.Orders[0].ID, "picker", "ship"); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			if action != "ship" {
				if err := s.out.Cancel(s.ctx, order.ID, "manager"); err != nil {
					t.Fatalf("cancelled history blocked untouched replacement: %v", err)
				}
				if row := s.reloadOrder(t, order.ID); row.Status != outmodel.OrderCancelled {
					t.Fatalf("order=%+v", row)
				}
			} else if row := s.reloadOrder(t, order.ID); row.Status != outmodel.OrderShipped {
				t.Fatalf("order=%+v", row)
			}
			old := s.reloadTask(t, original.ID)
			if old.Status != taskmodel.TaskCancelled || old.DoneQty != 2 {
				t.Fatalf("historical audit changed: %+v", old)
			}
			stock := s.inventory(t)
			want := 20
			if action == "ship" {
				want -= 5
			}
			if stock.StockQuantity != want || stock.AllocatedQty != 0 || stock.FrozenQty != 1 || stock.AvailableQty != want-1 {
				t.Fatalf("stock=%+v", stock)
			}
		})
	}
}

func TestCancelStillRejectsCurrentPickProgressAfterReplacement(t *testing.T) {
	s, _ := newClosureStack(t)
	o, task := s.newPickOrder(t, "ACTIVE-PICK", 5)
	if _, err := s.out.Pick(s.ctx, task.ID, 1, "picker", nil, "", "pick"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, "current pick cannot cancel", s.out.Cancel(s.ctx, o.ID, "manager"), errcode.ShipShippedForbidden)
}

func TestCancelReleasesInInventoryIDOrder(t *testing.T) {
	s := newPickRaceStack(t)
	// FIFO visits the original high ID before this later, low ID tuple.
	if err := s.db.WithContext(s.ctx).Create(&invmodel.Inventory{Base: modelbase.Base{ID: 10}, WarehouseID: s.warehouseID, LocationID: s.locationID, SKUID: s.skuID, BatchNo: "LATE", StockQuantity: 20, AvailableQty: 20, StockInTime: time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	o, err := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "ORDERED-CANCEL", Details: []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 25}}}, "manager")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.out.Submit(s.ctx, o.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.out.Approve(s.ctx, o.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	var locked []int64
	const callback = "test:cancel_inventory_order"
	if err := s.db.Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
		if q.Statement.Table == "wms_inventory" && q.Error == nil {
			if _, exists := q.Statement.Clauses["FOR"]; exists {
				if row, ok := q.Statement.Dest.(*invmodel.Inventory); ok {
					locked = append(locked, row.ID)
				}
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.db.Callback().Query().Remove(callback) }()
	if err := s.out.Cancel(s.ctx, o.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	if len(locked) != 2 || locked[0] != 10 || locked[1] <= locked[0] {
		t.Fatalf("release lock order=%v", locked)
	}
}
