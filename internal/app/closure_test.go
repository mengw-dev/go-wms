package app

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"gorm.io/gorm"

	invapi "gowms/internal/modules/inventory/api"
	invdto "gowms/internal/modules/inventory/dto"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

func newClosureStack(t *testing.T) (*pickRaceStack, *invservice.Service) {
	t.Helper()
	s := newWaveStack(t)
	if e := s.db.AutoMigrate(&outmodel.WorkException{}, &outmodel.CustomerReturn{}); e != nil {
		t.Fatal(e)
	}
	return s, invservice.New(invrepo.New(), pkgtx.New(s.db))
}

func TestClosurePartialShipmentDamageReallocateReturn(t *testing.T) {
	s, inv := newClosureStack(t)
	order, task := s.newPickOrder(t, "CLOSURE-PARTIAL", 10)
	if _, e := s.out.Pick(s.ctx, task.ID, 3, "picker", nil, "", "partial"); e != nil {
		t.Fatal(e)
	}
	req := &outdto.ExceptionRequest{TaskID: task.ID, ConfirmReturned: true, Kind: "DAMAGE", Quantity: 2, Reason: "包装破损"}
	ack, e := s.out.ReportException(s.ctx, req, "manager", "damage")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.out.ReportException(s.ctx, req, "manager", "damage"); e != nil {
		t.Fatal(e)
	}
	stock := s.inventory(t)
	if stock.StockQuantity != 20 || stock.AllocatedQty != 0 || stock.FrozenQty != 2 || stock.AvailableQty != 18 {
		t.Fatalf("damage=%+v", stock)
	}
	if o := s.reloadOrder(t, order.ID); o.Status != outmodel.OrderException || o.PickedQty != 0 || o.AllocatedQty != 0 {
		t.Fatalf("order=%+v", o)
	}
	if _, e = s.out.Pick(s.ctx, task.ID, 1, "picker", nil, "", "old-task"); e == nil {
		t.Fatal("cancelled task accepted")
	}
	if _, e = s.out.ResolveWorkException(s.ctx, ack.ObjectID, &outdto.ResolveExceptionRequest{Reason: "重新分配未发10件"}, "manager", "resolve"); e != nil {
		t.Fatal(e)
	}
	data, e := s.out.GetResponse(s.ctx, order.ID)
	if e != nil {
		t.Fatal(e)
	}
	var newID int64
	for _, row := range data.Tasks {
		if row.Status == "CREATED" {
			newID = row.ID
			if row.TargetQty != 10 {
				t.Fatalf("retry qty=%d", row.TargetQty)
			}
		}
	}
	if newID == 0 {
		t.Fatal("missing replacement task")
	}
	if _, e = s.out.Pick(s.ctx, newID, 10, "picker", nil, "", "replacement"); e != nil {
		t.Fatal(e)
	}
	stock = s.inventory(t)
	if stock.StockQuantity != 10 || stock.AvailableQty != 8 || stock.FrozenQty != 2 || stock.AllocatedQty != 0 {
		t.Fatalf("shipped=%+v", stock)
	}
	data, e = s.out.GetResponse(s.ctx, order.ID)
	if e != nil {
		t.Fatal(e)
	}
	var allocation int64
	for _, row := range data.Allocations {
		if row.PickedQty == 10 {
			allocation = row.ID
		}
	}
	ret := &outdto.CustomerReturnRequest{AllocationID: allocation, LocationID: s.locationID, GoodQty: 2, BadQty: 1, Reason: "客户验收退货"}
	for range 2 {
		if _, e = s.out.ReceiveCustomerReturn(s.ctx, ret, "receiver", "return"); e != nil {
			t.Fatal(e)
		}
	}
	stock = s.inventory(t)
	if stock.StockQuantity != 13 || stock.AvailableQty != 10 || stock.FrozenQty != 3 {
		t.Fatalf("return=%+v", stock)
	}
	ret.GoodQty = 8
	ret.BadQty = 0
	_, e = s.out.ReceiveCustomerReturn(s.ctx, ret, "receiver", "over-return")
	requireCode(t, "over return", e, errcode.ReturnNotAllowed)
	// Frozen stock cannot be counted away; failed adjustment must leave it intact.
	e = pkgtx.New(s.db).Tx(s.ctx, func(db *gorm.DB) error {
		_, e := inv.Adjust(s.ctx, db, &invapi.AdjustReq{InventoryID: stock.ID, NewStock: 2, OrderNo: "COUNT", Operator: "counter"})
		return e
	})
	requireCode(t, "count frozen floor", e, errcode.AdjustNotAllow)
}

func TestClosureShortageCloseAndIsolation(t *testing.T) {
	s, _ := newClosureStack(t)
	order, task := s.newPickOrder(t, "CLOSURE-SHORT", 5)
	req := &outdto.ExceptionRequest{TaskID: task.ID, ConfirmReturned: true, Kind: "SHORTAGE", Quantity: 3, Reason: "实物短缺"}
	ack, e := s.out.ReportException(s.ctx, req, "manager", "short")
	if e != nil {
		t.Fatal(e)
	}
	if stock := s.inventory(t); stock.StockQuantity != 17 || stock.AvailableQty != 17 || stock.AllocatedQty != 0 {
		t.Fatalf("stock=%+v", stock)
	}
	for _, tid := range []int64{0, raceTenantID + 1} {
		ctx := tenant.WithTenant(context.Background(), tid)
		rows, n, e := s.out.ListExceptions(ctx, &outdto.ClosureQuery{Page: 1, PageSize: 10})
		if e != nil || n != 0 || len(rows) != 0 {
			t.Fatalf("foreign list=%+v %d %v", rows, n, e)
		}
		if _, e = s.out.ResolveWorkException(ctx, ack.ObjectID, &outdto.ResolveExceptionRequest{Close: true, Reason: "越权"}, "manager", "foreign"); e == nil {
			t.Fatal("foreign resolve")
		}
	}
	if _, e = s.out.ResolveWorkException(s.ctx, ack.ObjectID, &outdto.ResolveExceptionRequest{Close: true, Reason: "客户取消剩余需求"}, "manager", "close"); e != nil {
		t.Fatal(e)
	}
	if o := s.reloadOrder(t, order.ID); o.Status != outmodel.OrderClosed || o.ExpectedQty != 5 || o.PickedQty != 0 {
		t.Fatalf("closed=%+v", o)
	}
}

func TestClosureConcurrentReturnCapAndRollback(t *testing.T) {
	s, _ := newClosureStack(t)
	o, task := s.newPickOrder(t, "CLOSURE-RETURNS", 5)
	if _, e := s.out.Pick(s.ctx, task.ID, 5, "picker", nil, "", "ship"); e != nil {
		t.Fatal(e)
	}
	data, e := s.out.GetResponse(s.ctx, o.ID)
	if e != nil {
		t.Fatal(e)
	}
	req := &outdto.CustomerReturnRequest{AllocationID: data.Allocations[0].ID, LocationID: s.locationID, GoodQty: 3, Reason: "客户退货"}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := s.out.ReceiveCustomerReturn(s.ctx, req, "receiver", fmt.Sprintf("return-%d", i))
			errs <- e
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		} else {
			requireCode(t, "cap", e, errcode.ReturnNotAllowed)
		}
	}
	if successes != 1 {
		t.Fatalf("successes=%d", successes)
	}
	if stock := s.inventory(t); stock.StockQuantity != 18 {
		t.Fatalf("over-returned=%+v", stock)
	}
	if e = s.db.Exec("CREATE TRIGGER fail_return BEFORE INSERT ON wms_customer_return FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='injected failure'").Error; e != nil {
		t.Fatal(e)
	}
	req.GoodQty = 1
	req.BadQty = 1
	if _, e = s.out.ReceiveCustomerReturn(s.ctx, req, "receiver", "rollback"); e == nil {
		t.Fatal("injection failed")
	}
	if stock := s.inventory(t); stock.StockQuantity != 18 || stock.FrozenQty != 0 {
		t.Fatalf("not rolled back=%+v", stock)
	}
}

func TestClosureFreezeVsAllocationAndUnfreeze(t *testing.T) {
	s, svc := newClosureStack(t)
	stock := s.inventory(t)
	req := &invdto.FreezeRequest{InventoryID: stock.ID, Quantity: 6, Reason: "质量待检"}
	for range 2 {
		if _, e := svc.FreezeStock(s.ctx, req, "freeze", "manager"); e != nil {
			t.Fatal(e)
		}
	}
	if stock = s.inventory(t); stock.AvailableQty != 14 || stock.FrozenQty != 6 {
		t.Fatalf("freeze=%+v", stock)
	}
	req.Quantity = 15
	_, e := svc.FreezeStock(s.ctx, req, "too-much", "manager")
	requireCode(t, "over freeze", e, errcode.FrozenNotEnough)
	req.Quantity = 3
	req.Unfreeze = true
	if _, e = svc.FreezeStock(s.ctx, req, "unfreeze", "manager"); e != nil {
		t.Fatal(e)
	}
	if stock = s.inventory(t); stock.AvailableQty != 17 || stock.FrozenQty != 3 {
		t.Fatalf("unfreeze=%+v", stock)
	}
}

func TestClosureFreezeRacesAllocationWithoutOverselling(t *testing.T) {
	s, svc := newClosureStack(t)
	stock := s.inventory(t)
	o, e := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "FREEZE-RACE", Details: []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 15}}}, "manager")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.out.Submit(s.ctx, o.ID); e != nil {
		t.Fatal(e)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, e := svc.FreezeStock(s.ctx, &invdto.FreezeRequest{InventoryID: stock.ID, Quantity: 10, Reason: "质量待检"}, "race-freeze", "manager")
		errs <- e
	}()
	go func() { defer wg.Done(); <-start; errs <- s.out.Approve(s.ctx, o.ID, "manager") }()
	close(start)
	wg.Wait()
	close(errs)
	successes := 0
	for e := range errs {
		if e == nil {
			successes++
		} else {
			code := errcode.From(e).Code
			if code != errcode.FrozenNotEnough.Code && code != errcode.AvailableNotEnough.Code {
				t.Fatal(e)
			}
		}
	}
	stock = s.inventory(t)
	if successes != 1 || stock.StockQuantity != 20 || stock.StockQuantity != stock.AvailableQty+stock.AllocatedQty+stock.FrozenQty || stock.AvailableQty < 0 {
		t.Fatalf("successes=%d stock=%+v", successes, stock)
	}
}

func TestClosureAbortPartiallyShippedWave(t *testing.T) {
	s, _ := newClosureStack(t)
	a, _ := s.newPickOrder(t, "ABORT-A", 4)
	b, taskB := s.newPickOrder(t, "ABORT-B", 5)
	ack, e := s.out.CreateWave(s.ctx, &outdto.CreateWaveReq{OrderIDs: []int64{a.ID, b.ID}}, "picker", "create")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.out.ClaimWave(s.ctx, ack.WaveID, "picker", "claim"); e != nil {
		t.Fatal(e)
	}
	data := waveDetail(t, s, ack.WaveID)
	line := data.Lines[0]
	if _, e = s.out.CollectWave(s.ctx, ack.WaveID, line.ID, outdto.WaveQuantityReq{Qty: 9, SKUCode: line.SKUCode, LocationCode: line.LocationCode, BatchNo: line.BatchNo}, "picker", "collect"); e != nil {
		t.Fatal(e)
	}
	for _, item := range data.Items {
		var slot string
		for _, o := range data.Orders {
			if o.ID == item.WaveOrderID {
				slot = o.SlotCode
			}
		}
		if _, e = s.out.SortWave(s.ctx, ack.WaveID, item.ID, outdto.WaveQuantityReq{Qty: item.TargetQty, SKUCode: line.SKUCode, BatchNo: line.BatchNo, SlotCode: slot}, "picker", fmt.Sprintf("sort-%d", item.ID)); e != nil {
			t.Fatal(e)
		}
	}
	for _, o := range data.Orders {
		if o.OrderID == a.ID {
			if _, e = s.out.ShipWaveOrder(s.ctx, ack.WaveID, o.ID, "picker", "ship-a"); e != nil {
				t.Fatal(e)
			}
		}
	}
	if _, e = s.out.AbortWorkWave(s.ctx, ack.WaveID, &outdto.AbortWaveRequest{Reason: "等待现场核对"}, "manager", "no-confirm"); e == nil {
		t.Fatal("unconfirmed abort")
	}
	req := &outdto.AbortWaveRequest{ConfirmReturned: true, Reason: "未发货实物已退回原位"}
	for range 2 {
		if _, e = s.out.AbortWorkWave(s.ctx, ack.WaveID, req, "manager", "abort"); e != nil {
			t.Fatal(e)
		}
	}
	data = waveDetail(t, s, ack.WaveID)
	if data.Wave.Status != outmodel.WaveAborted || data.Wave.AbortedBy != "manager" || data.Wave.AbortedAt == nil || s.reloadTask(t, taskB.ID).WaveID != 0 {
		t.Fatalf("abort=%+v", data.Wave)
	}
	if stock := s.inventory(t); stock.StockQuantity != 16 || stock.AllocatedQty != 5 || stock.AvailableQty != 11 {
		t.Fatalf("abort stock=%+v", stock)
	}
	for _, o := range data.Orders {
		if o.OrderID == b.ID {
			_, e = s.out.ShipWaveOrder(s.ctx, ack.WaveID, o.ID, "picker", "late-wave-ship")
			requireCode(t, "ended wave", e, errcode.WaveStatusWrong)
		}
	}
	if _, e = s.out.Pick(s.ctx, taskB.ID, 5, "picker", nil, "", "ordinary-b"); e != nil {
		t.Fatal(e)
	}
	if stock := s.inventory(t); stock.StockQuantity != 11 || stock.AllocatedQty != 0 {
		t.Fatalf("final=%+v", stock)
	}
}

func TestClosureKeepsShippedAllocationAndReturnsUnshippedPartialPicking(t *testing.T) {
	s, inv := newClosureStack(t)
	if e := pkgtx.New(s.db).Tx(s.ctx, func(db *gorm.DB) error {
		return inv.Increase(s.ctx, db, &invapi.IncreaseReq{WarehouseID: s.warehouseID, LocationID: s.locationID, SKUID: s.skuID, BatchNo: "SECOND", Quantity: 10, OrderNo: "SEED", Operator: "seed"})
	}); e != nil {
		t.Fatal(e)
	}
	order, e := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "MIXED-SHIPMENT", Details: []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 25}}}, "manager")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.out.Submit(s.ctx, order.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.out.Approve(s.ctx, order.ID, "manager"); e != nil {
		t.Fatal(e)
	}
	data, e := s.out.GetResponse(s.ctx, order.ID)
	if e != nil {
		t.Fatal(e)
	}
	var partial int64
	for _, task := range data.Tasks {
		qty := task.TargetQty
		if qty == 5 {
			qty = 2
			partial = task.ID
		}
		if _, e = s.out.Pick(s.ctx, task.ID, qty, "picker", nil, "", fmt.Sprintf("mixed-pick-%d", task.ID)); e != nil {
			t.Fatal(e)
		}
	}
	if partial == 0 {
		t.Fatal("missing second allocation")
	}
	ack, e := s.out.ReportException(s.ctx, &outdto.ExceptionRequest{TaskID: partial, Kind: "SHORTAGE", Quantity: 1, ConfirmReturned: true, Reason: "未发部分退回并核实短缺"}, "manager", "mixed-shortage")
	if e != nil {
		t.Fatal(e)
	}
	if o := s.reloadOrder(t, order.ID); o.PickedQty != 20 || o.AllocatedQty != 20 || o.Status != outmodel.OrderException {
		t.Fatalf("shipped prefix lost=%+v", o)
	}
	if _, e = s.out.ResolveWorkException(s.ctx, ack.ObjectID, &outdto.ResolveExceptionRequest{Close: true, Reason: "关闭未发5件"}, "manager", "mixed-close"); e != nil {
		t.Fatal(e)
	}
	rows, e := s.out.Returnable(s.ctx, order.ID)
	if e != nil || len(rows) != 1 || rows[0].ShippedQty != 20 {
		t.Fatalf("returnable=%+v %v", rows, e)
	}
	data, e = s.out.GetResponse(s.ctx, order.ID)
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range data.Allocations {
		if a.Status == outmodel.AllocCancelled {
			_, e = s.out.ReceiveCustomerReturn(s.ctx, &outdto.CustomerReturnRequest{AllocationID: a.ID, LocationID: s.locationID, GoodQty: 1, Reason: "不可把未发进度当成退货"}, "receiver", "not-shipped-return")
			requireCode(t, "unshipped allocation", e, errcode.ReturnNotAllowed)
		}
	}
}
