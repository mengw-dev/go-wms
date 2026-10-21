package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
)

func newWaveStack(t *testing.T) *pickRaceStack {
	t.Helper()
	s := newPickRaceStack(t)
	if err := s.db.AutoMigrate(&outmodel.Wave{}, &outmodel.WaveOrder{}, &outmodel.WaveLine{}, &outmodel.WaveItem{}); err != nil {
		t.Fatal(err)
	}
	return s
}

func waveDetail(t *testing.T, s *pickRaceStack, id int64) *outdto.WaveDetailResp {
	t.Helper()
	d, err := s.out.GetWave(s.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestWavePickingAndSowingShipment(t *testing.T) {
	s := newWaveStack(t)
	a, taskA := s.newPickOrder(t, "WAVE-A", 5)
	b, _ := s.newPickOrder(t, "WAVE-B", 3)
	preview, err := s.out.WaveCandidates(s.ctx, &outdto.WaveCandidateQuery{WarehouseID: s.warehouseID, Limit: 20})
	if err != nil || len(preview) != 2 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	ack, err := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{b.ID, a.ID}}, "picker", "create")
	if err != nil {
		t.Fatal(err)
	}
	// Exact replay remains valid after membership has changed.
	replay, err := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID, b.ID}}, "picker", "create")
	if err != nil || replay.WaveID != ack.WaveID {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
	_, err = s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID}}, "picker", "create")
	requireCode(t, "changed create", err, errcode.IdempotencyKeyReused)
	d := waveDetail(t, s, ack.WaveID)
	if len(d.Lines) != 1 || d.Lines[0].TargetQty != 8 || len(d.Orders) != 2 || len(d.Items) != 2 {
		t.Fatalf("detail=%+v", d)
	}
	_, err = s.out.Pick(s.ctx, taskA.ID, 1, "picker", nil, "", "legacy-pick")
	requireCode(t, "ordinary picking blocked", err, errcode.WaveOrderLocked)
	_, err = s.out.ClaimPickTask(s.ctx, taskA.ID, "picker", "legacy-claim")
	requireCode(t, "ordinary claim blocked", err, errcode.WaveOrderLocked)
	requireCode(t, "ordinary cancel blocked", s.out.Cancel(s.ctx, a.ID, "picker"), errcode.WaveOrderLocked)
	line := d.Lines[0]
	scan := outdto.WaveQuantityReq{Qty: 4, SKUCode: line.Barcode, LocationCode: line.LocationCode, BatchNo: line.BatchNo}
	_, err = s.out.CollectWave(s.ctx, ack.WaveID, line.ID, scan, "picker", "collect")
	requireCode(t, "claim required", err, errcode.WaveClaimRequired)
	if _, err = s.out.ClaimWave(s.ctx, ack.WaveID, "picker", "claim"); err != nil {
		t.Fatal(err)
	}
	_, err = s.out.ClaimWave(s.ctx, ack.WaveID, "other", "other-claim")
	requireCode(t, "exclusive owner", err, errcode.WaveClaimRequired)
	wrong := scan
	wrong.BatchNo = "WRONG"
	_, err = s.out.CollectWave(s.ctx, ack.WaveID, line.ID, wrong, "picker", "bad-scan")
	requireCode(t, "batch", err, errcode.WaveScanMismatch)
	for range 2 {
		if _, err = s.out.CollectWave(s.ctx, ack.WaveID, line.ID, scan, "picker", "collect"); err != nil {
			t.Fatal(err)
		}
	}
	if got := waveDetail(t, s, ack.WaveID).Lines[0].PickedQty; got != 4 {
		t.Fatalf("duplicate collect=%d", got)
	}
	_, err = s.out.CancelWave(s.ctx, ack.WaveID, "picker", "cancel-after-collect")
	requireCode(t, "cancel after collect", err, errcode.WaveStatusWrong)
	item := d.Items[0]
	sort := outdto.WaveQuantityReq{Qty: item.TargetQty, SKUCode: line.SKUCode, BatchNo: line.BatchNo, SlotCode: d.Orders[0].SlotCode}
	_, err = s.out.SortWave(s.ctx, ack.WaveID, item.ID, sort, "picker", "early-sort")
	requireCode(t, "partial collection", err, errcode.WaveStatusWrong)
	if _, err = s.out.CollectWave(s.ctx, ack.WaveID, line.ID, scan, "picker", "collect-rest"); err != nil {
		t.Fatal(err)
	}
	if inv := s.inventory(t); inv.StockQuantity != 20 || inv.AllocatedQty != 8 {
		t.Fatalf("collection consumed inventory: %+v", inv)
	}
	if s.reloadTask(t, taskA.ID).DoneQty != 0 {
		t.Fatal("collection advanced ordinary task before shipping")
	}
	_, err = s.out.ShipWaveOrder(s.ctx, ack.WaveID, d.Orders[0].ID, "picker", "early-ship")
	requireCode(t, "unsorted shipment", err, errcode.WaveNotSorted)
	for _, i := range d.Items {
		var member *outmodel.WaveOrder
		for _, o := range d.Orders {
			if o.ID == i.WaveOrderID {
				member = o
			}
		}
		if member == nil {
			t.Fatal("missing slot")
		}
		q := outdto.WaveQuantityReq{Qty: i.TargetQty, SKUCode: line.SKUCode, BatchNo: line.BatchNo, SlotCode: "WRONG"}
		_, err = s.out.SortWave(s.ctx, ack.WaveID, i.ID, q, "picker", fmt.Sprintf("wrong-slot-%d", i.ID))
		requireCode(t, "slot", err, errcode.WaveScanMismatch)
		q.SlotCode = member.SlotCode
		for range 2 {
			if _, err = s.out.SortWave(s.ctx, ack.WaveID, i.ID, q, "picker", fmt.Sprintf("sort-%d", i.ID)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if inv := s.inventory(t); inv.StockQuantity != 20 || inv.AllocatedQty != 8 {
		t.Fatalf("sort consumed inventory: %+v", inv)
	}
	if got := waveDetail(t, s, ack.WaveID).Wave.Status; got != outmodel.WaveReady {
		t.Fatalf("status=%s", got)
	}
	for _, o := range d.Orders {
		for n := range 2 { // Even a different key cannot ship a member twice.
			if _, err = s.out.ShipWaveOrder(s.ctx, ack.WaveID, o.ID, "picker", fmt.Sprintf("ship-%d-%d", o.ID, n)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if inv := s.inventory(t); inv.StockQuantity != 12 || inv.AllocatedQty != 0 || inv.AvailableQty != 12 {
		t.Fatalf("shipment inventory: %+v", inv)
	}
	if got := waveDetail(t, s, ack.WaveID).Wave.Status; got != outmodel.WaveCompleted {
		t.Fatalf("status=%s", got)
	}
	if s.reloadOrder(t, a.ID).Status != outmodel.OrderShipped || s.reloadOrder(t, b.ID).Status != outmodel.OrderShipped || s.reloadTask(t, taskA.ID).DoneQty != 5 {
		t.Fatal("original order/task not completed")
	}
	var count int64
	if err = s.db.WithContext(s.ctx).Table("wms_inventory_trans").Where("trans_type = 'SHIP'").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("SHIP flow count=%d err=%v", count, err)
	}
}

func TestWaveCancellationRestoresOrdinaryPickingAndIsolation(t *testing.T) {
	s := newWaveStack(t)
	a, task := s.newPickOrder(t, "CANCEL-WAVE", 2)
	ack, err := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID}}, "picker", "create")
	if err != nil {
		t.Fatal(err)
	}
	for _, tid := range []int64{0, raceTenantID + 1} {
		ctx := tenant.WithTenant(context.Background(), tid)
		_, err = s.out.GetWave(ctx, ack.WaveID)
		requireCode(t, "foreign read", err, errcode.WaveNotFound)
		_, err = s.out.ClaimWave(ctx, ack.WaveID, "picker", "claim")
		requireCode(t, "foreign claim", err, errcode.WaveNotFound)
		rows, total, listErr := s.out.ListWaves(ctx, &outdto.WaveQuery{Page: 1, PageSize: 10})
		if listErr != nil || total != 0 || len(rows) != 0 {
			t.Fatalf("foreign list: %v %d %v", rows, total, listErr)
		}
	}
	if _, err = s.out.CancelWave(s.ctx, ack.WaveID, "picker", "cancel"); err != nil {
		t.Fatal(err)
	}
	if s.reloadTask(t, task.ID).WaveID != 0 || s.inventory(t).AllocatedQty != 2 {
		t.Fatal("cancel changed allocation or retained membership")
	}
	if _, err = s.out.Pick(s.ctx, task.ID, 2, "picker", nil, "", "ordinary-after-cancel"); err != nil {
		t.Fatal(err)
	}
	if inv := s.inventory(t); inv.StockQuantity != 18 || inv.AllocatedQty != 0 {
		t.Fatalf("legacy inventory=%+v", inv)
	}
}

func TestWaveConcurrentCommandsReadAfterHeaderLock(t *testing.T) {
	s := newWaveStack(t)
	a, _ := s.newPickOrder(t, "CONCURRENT-WAVE", 2)
	ack, err := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID}}, "picker", "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.out.ClaimWave(s.ctx, ack.WaveID, "picker", "claim"); err != nil {
		t.Fatal(err)
	}
	d := waveDetail(t, s, ack.WaveID)
	line := d.Lines[0]
	// Pause the first update while the second command passes replay and waits
	// on the header. Its quantity check must see the first committed quantity.
	gate := &txGate{arrived: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(gate.open)
	if err = s.db.Callback().Update().Before("gorm:update").Register("wave-collect-gate", func(db *gorm.DB) {
		if db.Statement.Table == "wms_wave_line" {
			gate.once.Do(func() {
				close(gate.arrived)
				select {
				case <-gate.release:
				case <-db.Statement.Context.Done():
					_ = db.AddError(db.Statement.Context.Err())
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Callback().Update().Remove("wave-collect-gate") })
	results := make(chan error, 2)
	q := outdto.WaveQuantityReq{Qty: 2, SKUCode: line.SKUCode, LocationCode: line.LocationCode, BatchNo: line.BatchNo}
	s.run(func() { _, e := s.out.CollectWave(s.ctx, ack.WaveID, line.ID, q, "picker", "first"); results <- e })
	waitClosed(t, "first update", gate.arrived)
	second := make(chan struct{})
	s.run(func() {
		close(second)
		_, e := s.out.CollectWave(s.ctx, ack.WaveID, line.ID, q, "picker", "second")
		results <- e
	})
	waitClosed(t, "second launched", second)
	// Observe the actual database lock waiter rather than a scheduling sleep.
	deadline := time.Now().Add(3 * time.Second)
	for {
		var n int64
		if e := s.db.Raw("SELECT COUNT(*) FROM performance_schema.data_lock_waits").Scan(&n).Error; e != nil {
			t.Fatal(e)
		}
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no header lock waiter")
		}
		time.Sleep(10 * time.Millisecond)
	}
	gate.open()
	var ok, rejected int
	for range 2 {
		e := waitError(t, "collect", results)
		switch {
		case e == nil:
			ok++
		case errors.Is(e, errcode.WaveStatusWrong) || errors.Is(e, errcode.WaveQuantityWrong):
			rejected++
		default:
			t.Fatal(e)
		}
	}
	if ok != 1 || rejected != 1 || waveDetail(t, s, ack.WaveID).Lines[0].PickedQty != 2 {
		t.Fatalf("ok=%d rejected=%d", ok, rejected)
	}
}

func TestWaveConcurrentMembership(t *testing.T) {
	s := newWaveStack(t)
	a, _ := s.newPickOrder(t, "MEMBERSHIP-WAVE", 1)
	var start sync.WaitGroup
	start.Add(1)
	results := make(chan error, 2)
	for n := range 2 {
		s.run(func() {
			start.Wait()
			_, e := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID}}, "picker", fmt.Sprintf("create-%d", n))
			results <- e
		})
	}
	start.Done()
	var ok, rejected int
	for range 2 {
		e := waitError(t, "membership", results)
		switch {
		case e == nil:
			ok++
		case errors.Is(e, errcode.WaveOrderInvalid):
			rejected++
		default:
			t.Fatal(e)
		}
	}
	if ok != 1 || rejected != 1 {
		t.Fatalf("ok=%d rejected=%d", ok, rejected)
	}
}

func TestWaveShipmentFailureRollsBackAllBusinessWrites(t *testing.T) {
	s := newWaveStack(t)
	a, task := s.newPickOrder(t, "ROLLBACK-WAVE", 2)
	ack, err := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID}}, "picker", "create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.out.ClaimWave(s.ctx, ack.WaveID, "picker", "claim"); err != nil {
		t.Fatal(err)
	}
	d := waveDetail(t, s, ack.WaveID)
	line, item := d.Lines[0], d.Items[0]
	q := outdto.WaveQuantityReq{Qty: 2, SKUCode: line.SKUCode, LocationCode: line.LocationCode, BatchNo: line.BatchNo, SlotCode: d.Orders[0].SlotCode}
	if _, err = s.out.CollectWave(s.ctx, ack.WaveID, line.ID, q, "picker", "collect"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.out.SortWave(s.ctx, ack.WaveID, item.ID, q, "picker", "sort"); err != nil {
		t.Fatal(err)
	}
	injected := errors.New("idempotency write failure")
	if err = s.db.Callback().Create().Before("gorm:create").Register("wave-idempotency-failure", func(db *gorm.DB) {
		if db.Statement.Table == "wms_idempotency" {
			_ = db.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.db.Callback().Create().Remove("wave-idempotency-failure") })
	_, err = s.out.ShipWaveOrder(s.ctx, ack.WaveID, d.Orders[0].ID, "picker", "ship")
	if !errors.Is(err, injected) {
		t.Fatalf("expected injected failure, got %v", err)
	}
	if inv := s.inventory(t); inv.StockQuantity != 20 || inv.AllocatedQty != 2 || s.reloadTask(t, task.ID).DoneQty != 0 || s.reloadOrder(t, a.ID).Status != outmodel.OrderPicking {
		t.Fatalf("partial shipment: %+v", inv)
	}
	if got := waveDetail(t, s, ack.WaveID); got.Orders[0].ShippedAt != nil || got.Wave.Status != outmodel.WaveReady {
		t.Fatal("shipment marker escaped rollback")
	}
	if err = s.db.Callback().Create().Remove("wave-idempotency-failure"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.out.ShipWaveOrder(s.ctx, ack.WaveID, d.Orders[0].ID, "picker", "ship"); err != nil {
		t.Fatal(err)
	}
	if s.inventory(t).StockQuantity != 18 {
		t.Fatal("retry did not commit")
	}
}
