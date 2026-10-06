package app

// 拣货 ↔ 取消在任务行锁上的三条竞态测试（真实 MySQL + 真实事务）：
//   - 取消先到：取消持有任务行锁未提交时拣货排队，取消提交后拣货被拒（任务已取消）；
//   - 拣货先到：拣货持有任务行锁未提交时取消排队，拣货提交后取消看到已开工被拒；
//   - 同时到：两者并发，恰好一方成功，另一方按业务规则被拒，且无死锁/超时。
//
// 另附 PDA 三项新契约的集成测试：领取凭证、强制扫码、幂等重放不重复扣减。

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	invapi "gowms/internal/modules/inventory/api"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invsrvc "gowms/internal/modules/inventory/service"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	outrepo "gowms/internal/modules/outbound/repository"
	outservice "gowms/internal/modules/outbound/service"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

const (
	raceTenantID = int64(20001)
	raceSeedQty  = 20
	raceTimeout  = 15 * time.Second
)

// pickRaceStack 与 app.New 相同的组装方式，用真实 MySQL 构建拣货链路。
type pickRaceStack struct {
	db    *gorm.DB
	out   *outservice.Service
	tasks *taskservice.Service
	ctx   context.Context

	warehouseID int64
	skuID       int64
	locationID  int64
}

func newPickRaceStack(t *testing.T) *pickRaceStack {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn,
		&basicmodel.Warehouse{}, &basicmodel.SKU{}, &basicmodel.Location{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{},
		&outmodel.ShipmentOrder{}, &outmodel.ShipmentOrderDetail{}, &outmodel.Allocation{},
		&taskmodel.Task{}, &idempotency.Record{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}

	tm := pkgtx.New(db)
	invSvc := invsrvc.New(invrepo.New(), tm)
	basicSvc := basicservice.New(basicrepo.New(), tm, nil, invSvc, config.LimitsConfig{})
	taskSvc := taskservice.New(taskrepo.New(), db)
	outSvc := outservice.New(outrepo.New(), tm, orderno.New(nil), basicSvc, invSvc, taskSvc, config.LimitsConfig{})

	s := &pickRaceStack{db: db, out: outSvc, tasks: taskSvc, ctx: tenant.WithTenant(context.Background(), raceTenantID)}
	s.warehouseID = snowflake.Next()
	s.skuID = snowflake.Next()
	s.locationID = snowflake.Next()
	if err := db.WithContext(s.ctx).Create(&basicmodel.Warehouse{
		Base: modelbase.Base{ID: s.warehouseID}, Code: fmt.Sprintf("RACE-WH-%d", s.warehouseID), Name: "race", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(s.ctx).Create(&basicmodel.SKU{
		Base: modelbase.Base{ID: s.skuID}, Code: fmt.Sprintf("RACE-SKU-%d", s.skuID),
		Barcode: fmt.Sprintf("RACE-BC-%d", s.skuID), Name: "race", Unit: "件", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(s.ctx).Create(&basicmodel.Location{
		Base: modelbase.Base{ID: s.locationID}, WarehouseID: s.warehouseID,
		Code: fmt.Sprintf("RACE-LOC-%d", s.locationID), Status: basicmodel.LocationStatusIdle,
	}).Error; err != nil {
		t.Fatal(err)
	}
	// 用真实入库建立库存行（同租户上下文），保证 FIFO 分配可用。
	if err := tm.Tx(s.ctx, func(tx *gorm.DB) error {
		return invSvc.Increase(s.ctx, tx, &invapi.IncreaseReq{
			WarehouseID: s.warehouseID, LocationID: s.locationID, SKUID: s.skuID,
			BatchNo: "RACE-B1", Quantity: raceSeedQty, OrderNo: "RACE-SEED", Operator: "tester",
		})
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

// newPickOrder 建单 → 提交 → 审核分配（真实 FIFO 分配 + 任务生成），返回订单与拣货任务。
func (s *pickRaceStack) newPickOrder(t *testing.T, bizNo string, qty int) (*outmodel.ShipmentOrder, *taskmodel.Task) {
	t.Helper()
	order, err := s.out.Create(s.ctx, &outdto.CreateOrderReq{
		WarehouseID: s.warehouseID, BizOrderNo: bizNo,
		Details: []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: qty}},
	}, "tester")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if err := s.out.Submit(s.ctx, order.ID); err != nil {
		t.Fatalf("submit order: %v", err)
	}
	if err := s.out.Approve(s.ctx, order.ID, "tester"); err != nil {
		t.Fatalf("approve order: %v", err)
	}
	list, total, err := s.tasks.List(s.ctx, order.ID, string(taskmodel.TaskPick), "", "", 1, 10)
	if err != nil || total != 1 {
		t.Fatalf("list pick tasks: total=%d err=%v", total, err)
	}
	return order, list[0]
}

func (s *pickRaceStack) reloadOrder(t *testing.T, id int64) *outmodel.ShipmentOrder {
	t.Helper()
	var o outmodel.ShipmentOrder
	if err := s.db.WithContext(s.ctx).First(&o, id).Error; err != nil {
		t.Fatal(err)
	}
	return &o
}

func (s *pickRaceStack) reloadTask(t *testing.T, id int64) *taskmodel.Task {
	t.Helper()
	var task taskmodel.Task
	if err := s.db.WithContext(s.ctx).First(&task, id).Error; err != nil {
		t.Fatal(err)
	}
	return &task
}

func (s *pickRaceStack) reloadAllocation(t *testing.T, id int64) *outmodel.Allocation {
	t.Helper()
	var a outmodel.Allocation
	if err := s.db.WithContext(s.ctx).First(&a, id).Error; err != nil {
		t.Fatal(err)
	}
	return &a
}

// inventory 返回仓库+SKU 的库存行，用于校验三数量不变量。
func (s *pickRaceStack) inventory(t *testing.T) *invmodel.Inventory {
	t.Helper()
	var inv invmodel.Inventory
	if err := s.db.WithContext(s.ctx).
		Where("warehouse_id = ? AND sku_id = ?", s.warehouseID, s.skuID).First(&inv).Error; err != nil {
		t.Fatal(err)
	}
	return &inv
}

func waitError(t *testing.T, label string, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(raceTimeout):
		t.Fatalf("%s: timed out (possible deadlock)", label)
		return nil
	}
}

func waitClosed(t *testing.T, label string, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(raceTimeout):
		t.Fatalf("%s: timed out", label)
	}
}

func requireCode(t *testing.T, label string, err error, want *errcode.Error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected code %d, got nil", label, want.Code)
	}
	if got := errcode.From(err).Code; got != want.Code {
		t.Fatalf("%s: code=%d want=%d err=%v", label, got, want.Code, err)
	}
}

// txGate 在匹配表的下一条 UPDATE 前暂停目标事务：
// arrived 关闭表示「目标事务已持有任务行锁且尚未提交」，release 关闭后放行。
type txGate struct {
	once    sync.Once
	arrived chan struct{}
	release chan struct{}
}

func newTaskUpdateGate(t *testing.T, db *gorm.DB, name string) *txGate {
	t.Helper()
	gate := &txGate{arrived: make(chan struct{}), release: make(chan struct{})}
	if err := db.Callback().Update().Before("gorm:update").Register(name, func(query *gorm.DB) {
		if query.Statement == nil || query.Statement.Table != "wms_task" {
			return
		}
		gate.once.Do(func() {
			close(gate.arrived)
			<-gate.release
		})
	}); err != nil {
		t.Fatal(err)
	}
	return gate
}

// signalTaskRowLock 在按单据锁定任务行（SELECT ... FOR UPDATE）的查询下发前关闭信号。
func signalTaskRowLock(t *testing.T, db *gorm.DB, name string, ch chan struct{}) {
	t.Helper()
	var once sync.Once
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(query *gorm.DB) {
		if _, ok := query.Statement.Dest.(*[]*taskmodel.Task); ok {
			once.Do(func() { close(ch) })
		}
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPickCancelRaceCancelFirst 取消先到：取消持锁未提交时拣货排队，
// 取消提交后拣货必须看到「任务已取消」并被拒，不产生任何拣货写入。
func TestPickCancelRaceCancelFirst(t *testing.T) {
	stack := newPickRaceStack(t)
	order, task := stack.newPickOrder(t, "RACE-CANCEL-FIRST", 2)
	gate := newTaskUpdateGate(t, stack.db, "race:cancel_first")

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- stack.out.Cancel(stack.ctx, order.ID, "tester") }()
	waitClosed(t, "cancel holds task rows", gate.arrived)

	pickDone := make(chan error, 1)
	go func() { _, err := stack.out.Pick(stack.ctx, task.ID, 2, "picker", nil, "", ""); pickDone <- err }()
	close(gate.release)

	if err := waitError(t, "cancel", cancelDone); err != nil {
		t.Fatalf("cancel should win: %v", err)
	}
	requireCode(t, "pick after cancel", waitError(t, "pick", pickDone), errcode.TaskStatusWrong)

	gotOrder := stack.reloadOrder(t, order.ID)
	if gotOrder.Status != outmodel.OrderCancelled || gotOrder.PickedQty != 0 {
		t.Fatalf("order after cancel-first: status=%s picked=%d", gotOrder.Status, gotOrder.PickedQty)
	}
	gotTask := stack.reloadTask(t, task.ID)
	if gotTask.Status != taskmodel.TaskCancelled || gotTask.DoneQty != 0 {
		t.Fatalf("task after cancel-first: status=%s done=%d", gotTask.Status, gotTask.DoneQty)
	}
	gotAlloc := stack.reloadAllocation(t, task.AllocationID)
	if gotAlloc.Status != outmodel.AllocCancelled || gotAlloc.PickedQty != 0 {
		t.Fatalf("allocation after cancel-first: status=%s picked=%d", gotAlloc.Status, gotAlloc.PickedQty)
	}
	inv := stack.inventory(t)
	if inv.StockQuantity != raceSeedQty || inv.AvailableQty != raceSeedQty || inv.AllocatedQty != 0 {
		t.Fatalf("inventory after cancel-first: %+v", inv)
	}
}

// TestPickCancelRacePickFirst 拣货先到：拣货持锁未提交时取消排队；
// 拣货先读到「未开工」的取消必须被任务行锁纠正，看到已开工后拒绝取消。
func TestPickCancelRacePickFirst(t *testing.T) {
	stack := newPickRaceStack(t)
	order, task := stack.newPickOrder(t, "RACE-PICK-FIRST", 2)
	gate := newTaskUpdateGate(t, stack.db, "race:pick_first")
	lockAttempt := make(chan struct{})
	signalTaskRowLock(t, stack.db, "race:pick_first_lock", lockAttempt)

	pickDone := make(chan error, 1)
	go func() { _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", nil, "", ""); pickDone <- err }()
	waitClosed(t, "pick holds task row", gate.arrived)

	cancelDone := make(chan error, 1)
	go func() { cancelDone <- stack.out.Cancel(stack.ctx, order.ID, "tester") }()
	waitClosed(t, "cancel waits on task row", lockAttempt)
	close(gate.release)

	if err := waitError(t, "pick", pickDone); err != nil {
		t.Fatalf("pick should win: %v", err)
	}
	requireCode(t, "cancel after pick", waitError(t, "cancel", cancelDone), errcode.ShipShippedForbidden)

	gotOrder := stack.reloadOrder(t, order.ID)
	if gotOrder.Status != outmodel.OrderPicking || gotOrder.PickedQty != 1 {
		t.Fatalf("order after pick-first: status=%s picked=%d", gotOrder.Status, gotOrder.PickedQty)
	}
	gotTask := stack.reloadTask(t, task.ID)
	if gotTask.Status != taskmodel.TaskInProgress || gotTask.DoneQty != 1 {
		t.Fatalf("task after pick-first: status=%s done=%d", gotTask.Status, gotTask.DoneQty)
	}
	gotAlloc := stack.reloadAllocation(t, task.AllocationID)
	if gotAlloc.Status != outmodel.AllocAllocated || gotAlloc.PickedQty != 1 {
		t.Fatalf("allocation after pick-first: status=%s picked=%d", gotAlloc.Status, gotAlloc.PickedQty)
	}
	inv := stack.inventory(t)
	if inv.StockQuantity != raceSeedQty || inv.AvailableQty != raceSeedQty-2 || inv.AllocatedQty != 2 {
		t.Fatalf("inventory after pick-first: %+v", inv)
	}
}

// TestPickCancelRaceSimultaneous 同时到：两者并发执行，恰好一方成功；
// 断言无死锁/超时，且最终状态与唯一次胜者一致（三数量不变量恒成立）。
func TestPickCancelRaceSimultaneous(t *testing.T) {
	stack := newPickRaceStack(t)
	shippedTotal := 0
	for i := range 3 {
		order, task := stack.newPickOrder(t, fmt.Sprintf("RACE-BOTH-%d", i), 2)
		barrier := make(chan struct{})
		var wg sync.WaitGroup
		var pickErr, cancelErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-barrier
			_, pickErr = stack.out.Pick(stack.ctx, task.ID, 2, "picker", nil, "", "")
		}()
		go func() {
			defer wg.Done()
			<-barrier
			cancelErr = stack.out.Cancel(stack.ctx, order.ID, "tester")
		}()
		close(barrier)
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		waitClosed(t, "pick/cancel round", done)

		switch {
		case pickErr == nil && cancelErr != nil:
			// 拣货整单胜出：取消被业务规则拒绝。
			if code := errcode.From(cancelErr).Code; code != errcode.ShipShippedForbidden.Code && code != errcode.ShipOrderStatusWrong.Code {
				t.Fatalf("round %d: unexpected cancel error: %v", i, cancelErr)
			}
			shippedTotal += 2
			gotOrder := stack.reloadOrder(t, order.ID)
			if gotOrder.Status != outmodel.OrderShipped || gotOrder.PickedQty != 2 {
				t.Fatalf("round %d: order=%s picked=%d", i, gotOrder.Status, gotOrder.PickedQty)
			}
			if gotTask := stack.reloadTask(t, task.ID); gotTask.Status != taskmodel.TaskCompleted || gotTask.DoneQty != 2 {
				t.Fatalf("round %d: task=%s done=%d", i, gotTask.Status, gotTask.DoneQty)
			}
			if gotAlloc := stack.reloadAllocation(t, task.AllocationID); gotAlloc.Status != outmodel.AllocPicked {
				t.Fatalf("round %d: allocation=%s", i, gotAlloc.Status)
			}
		case cancelErr == nil && pickErr != nil:
			// 取消胜出：拣货被业务规则拒绝，库存回到未分配状态。
			requireCode(t, fmt.Sprintf("round %d pick", i), pickErr, errcode.TaskStatusWrong)
			gotOrder := stack.reloadOrder(t, order.ID)
			if gotOrder.Status != outmodel.OrderCancelled || gotOrder.PickedQty != 0 {
				t.Fatalf("round %d: order=%s picked=%d", i, gotOrder.Status, gotOrder.PickedQty)
			}
			if gotTask := stack.reloadTask(t, task.ID); gotTask.Status != taskmodel.TaskCancelled || gotTask.DoneQty != 0 {
				t.Fatalf("round %d: task=%s done=%d", i, gotTask.Status, gotTask.DoneQty)
			}
			if gotAlloc := stack.reloadAllocation(t, task.AllocationID); gotAlloc.Status != outmodel.AllocCancelled {
				t.Fatalf("round %d: allocation=%s", i, gotAlloc.Status)
			}
		default:
			t.Fatalf("round %d: exactly one side must win: pick=%v cancel=%v", i, pickErr, cancelErr)
		}
		// 终态不变量：已分配清零，可用量等于总库存减去已发货量。
		inv := stack.inventory(t)
		if inv.StockQuantity != raceSeedQty-shippedTotal || inv.AvailableQty != inv.StockQuantity || inv.AllocatedQty != 0 {
			t.Fatalf("round %d: inventory=%+v shipped=%d", i, inv, shippedTotal)
		}
	}
}

// TestPDAClaimStrictPickAndIdempotency 覆盖 PDA 三项契约：
// 领取凭证校验、强制扫码（后台宽松语义不受影响）、幂等重放不重复扣减、租约过期可被接手。
func TestPDAClaimStrictPickAndIdempotency(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "PDA-CLAIM-1", 2)

	// 未领取的凭证提交 → 拒绝
	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", nil, "bogus-token", ""); errcode.From(err).Code != errcode.TaskClaimMismatch.Code {
		t.Fatalf("pick with bogus token: %v", err)
	}
	// 领取任务 → 返回凭证与租约
	claim, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "picker")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claim.ClaimToken == "" || !claim.LeaseExpireAt.After(time.Now()) || claim.RemainingQty != 2 {
		t.Fatalf("claim result=%+v", claim)
	}
	// 强制扫码：缺库位
	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker",
		&outservice.PickScan{Strict: true}, claim.ClaimToken, ""); errcode.From(err).Code != errcode.PickLocationRequired.Code {
		t.Fatalf("strict pick without location: %v", err)
	}
	// 强制扫码：库位不符
	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker",
		&outservice.PickScan{Strict: true, LocationCode: "WRONG"}, claim.ClaimToken, ""); errcode.From(err).Code != errcode.PickLocationMismatch.Code {
		t.Fatalf("strict pick with wrong location: %v", err)
	}
	// 强制扫码：任务有批次但未扫批次
	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker",
		&outservice.PickScan{Strict: true, LocationCode: task.LocationCode}, claim.ClaimToken, ""); errcode.From(err).Code != errcode.PickBatchRequired.Code {
		t.Fatalf("strict pick without batch: %v", err)
	}
	// 正常拣货（带凭证 + 正确库位/批次 + 幂等键）
	scan := &outservice.PickScan{Strict: true, LocationCode: task.LocationCode, BatchNo: task.BatchNo}
	first, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, claim.ClaimToken, "idem-key-1")
	if err != nil {
		t.Fatalf("strict pick: %v", err)
	}
	if first.TaskStatus != taskmodel.TaskInProgress || first.DoneQty != 1 || first.RemainingQty != 1 {
		t.Fatalf("first result=%+v", first)
	}
	// 同 key 重放：回放首次快照，不得重复扣减
	replay, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, claim.ClaimToken, "idem-key-1")
	if err != nil || replay.DoneQty != first.DoneQty || replay.RemainingQty != first.RemainingQty {
		t.Fatalf("replay result=%+v err=%v", replay, err)
	}
	// 同 key 不同内容 → 409（客户端误用，不可重试）
	if _, err := stack.out.Pick(stack.ctx, task.ID, 2, "picker", scan, claim.ClaimToken, "idem-key-1"); errcode.From(err).Code != errcode.IdempotencyKeyReused.Code {
		t.Fatalf("reused key: %v", err)
	}
	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 1 {
		t.Fatalf("task done=%d want=1", gotTask.DoneQty)
	}
	if gotAlloc := stack.reloadAllocation(t, task.AllocationID); gotAlloc.PickedQty != 1 || gotAlloc.Status != outmodel.AllocAllocated {
		t.Fatalf("allocation=%s picked=%d", gotAlloc.Status, gotAlloc.PickedQty)
	}
	inv := stack.inventory(t)
	if inv.StockQuantity != raceSeedQty || inv.AvailableQty != raceSeedQty-2 || inv.AllocatedQty != 2 {
		t.Fatalf("inventory after replays: %+v", inv)
	}
	// 他人领取未过期租约 → 冲突
	if _, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "other"); errcode.From(err).Code != errcode.TaskClaimConflict.Code {
		t.Fatalf("claim busy task: %v", err)
	}
	// 租约过期：旧凭证被拒绝；他人可接手并换发新凭证
	if err := stack.db.WithContext(stack.ctx).Model(&taskmodel.Task{}).
		Where("id = ?", task.ID).Update("lease_expire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, claim.ClaimToken, ""); errcode.From(err).Code != errcode.TaskLeaseExpired.Code {
		t.Fatalf("expired lease pick: %v", err)
	}
	second, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "other")
	if err != nil || second.ClaimToken == claim.ClaimToken {
		t.Fatalf("re-claim expired lease: result=%+v err=%v", second, err)
	}
	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, claim.ClaimToken, ""); errcode.From(err).Code != errcode.TaskClaimMismatch.Code {
		t.Fatalf("stale token after re-claim: %v", err)
	}
}
