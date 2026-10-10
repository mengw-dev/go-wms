package service

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invsrvc "gowms/internal/modules/inventory/service"
	taskapi "gowms/internal/modules/task/api"
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
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func putawayTestDSN() string {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	return dsn
}

// putawayIdemStack 与 app.New 相同的上架链路组装（真实 MySQL）。
type putawayIdemStack struct {
	db        *gorm.DB
	s         *Service
	ctx       context.Context
	order     *model.ReceiptOrder
	task      *taskmodel.Task
	location  int64
	location2 int64
}

func newPutawayIdemStack(t *testing.T, targetQty int) *putawayIdemStack {
	t.Helper()
	db := testutil.OpenIsolatedMySQL(t, putawayTestDSN(),
		&model.ReceiptOrder{}, &model.ReceiptOrderDetail{}, &taskmodel.Task{}, &idempotency.Record{},
		&basicmodel.Warehouse{}, &basicmodel.SKU{}, &basicmodel.Location{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithTenant(context.Background(), 31)
	tm := tx.New(db)
	invSvc := invsrvc.New(invrepo.New(), tm)
	basicSvc := basicservice.New(basicrepo.New(), tm, nil, invSvc, config.LimitsConfig{})
	taskSvc := taskservice.New(taskrepo.New(), db)
	s := &Service{
		repo: repository.New(), tm: tm, no: orderno.New(nil),
		basic: basicSvc, inv: invSvc, taskAPI: taskSvc,
	}

	warehouseID := snowflake.Next()
	skuID := snowflake.Next()
	locationID := snowflake.Next()
	location2ID := snowflake.Next()
	if err := db.WithContext(ctx).Create(&basicmodel.Warehouse{
		Base: modelbase.Base{ID: warehouseID}, Code: "PA-WH", Name: "putaway", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&basicmodel.SKU{
		Base: modelbase.Base{ID: skuID}, Code: "PA-SKU", Barcode: "PA-BC", Name: "putaway", Unit: "件", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, loc := range []*basicmodel.Location{
		{Base: modelbase.Base{ID: locationID}, WarehouseID: warehouseID, Code: "PA-L1", Status: basicmodel.LocationStatusIdle},
		{Base: modelbase.Base{ID: location2ID}, WarehouseID: warehouseID, Code: "PA-L2", Status: basicmodel.LocationStatusIdle},
	} {
		if err := db.WithContext(ctx).Create(loc).Error; err != nil {
			t.Fatal(err)
		}
	}

	order := &model.ReceiptOrder{OrderNo: "PA-IDEM", Status: model.OrderPutaway, WarehouseID: warehouseID, ExpectedQty: targetQty}
	detail := &model.ReceiptOrderDetail{SKUID: skuID, ExpectedQty: targetQty, BatchNo: "B1"}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.repo.CreateOrder(tx, order, []*model.ReceiptOrderDetail{detail}); err != nil {
			return err
		}
		return taskSvc.Create(ctx, tx, []*taskapi.CreateTask{{
			TaskType: taskmodel.TaskPutaway, OrderID: order.ID, OrderNo: order.OrderNo,
			DetailID: detail.ID, SKUID: skuID, WarehouseID: warehouseID, TargetQty: targetQty,
		}})
	}); err != nil {
		t.Fatal(err)
	}
	var task taskmodel.Task
	if err := db.WithContext(ctx).Where("order_id = ? AND task_type = ?", order.ID, taskmodel.TaskPutaway).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	return &putawayIdemStack{db: db, s: s, ctx: ctx, order: order, task: &task, location: locationID, location2: location2ID}
}

func (st *putawayIdemStack) inventory(t *testing.T) *invmodel.Inventory {
	t.Helper()
	var inv invmodel.Inventory
	if err := st.db.WithContext(st.ctx).First(&inv).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &invmodel.Inventory{}
		}
		t.Fatal(err)
	}
	return &inv
}

func (st *putawayIdemStack) transCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := st.db.WithContext(st.ctx).Model(&invmodel.InventoryTrans{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func (st *putawayIdemStack) reloadTask(t *testing.T) *taskmodel.Task {
	t.Helper()
	var task taskmodel.Task
	if err := st.db.WithContext(st.ctx).First(&task, st.task.ID).Error; err != nil {
		t.Fatal(err)
	}
	return &task
}

func (st *putawayIdemStack) reloadLocation(t *testing.T) *basicmodel.Location {
	t.Helper()
	var loc basicmodel.Location
	if err := st.db.WithContext(st.ctx).First(&loc, st.location).Error; err != nil {
		t.Fatal(err)
	}
	return &loc
}

// TestPutawayIdempotencyReplay 上架请求级幂等：
// 同 key 重试不重复增加库存/流水/任务进度；新 key 同参数是另一次合法上架；
// 完成后重放不重复写入；同 key 改数量/库位返回 409。
func TestPutawayIdempotencyReplay(t *testing.T) {
	st := newPutawayIdemStack(t, 10)

	// 第一次上架 4 件
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 4, "test", "pa-key-1"); err != nil {
		t.Fatalf("first putaway: %v", err)
	}
	if inv := st.inventory(t); inv.StockQuantity != 4 {
		t.Fatalf("stock=%d want=4", inv.StockQuantity)
	}
	if n := st.transCount(t); n != 1 {
		t.Fatalf("trans=%d want=1", n)
	}
	// 同 key 同内容重试：回放空成功，库存/流水/进度都不变
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 4, "test", "pa-key-1"); err != nil {
		t.Fatalf("replay putaway: %v", err)
	}
	if inv := st.inventory(t); inv.StockQuantity != 4 {
		t.Fatalf("stock after replay=%d want=4", inv.StockQuantity)
	}
	if n := st.transCount(t); n != 1 {
		t.Fatalf("trans after replay=%d want=1", n)
	}
	if gotTask := st.reloadTask(t); gotTask.DoneQty != 4 {
		t.Fatalf("task done after replay=%d want=4", gotTask.DoneQty)
	}
	// 同 key 改内容（数量/库位）→ 409
	for _, tc := range []struct {
		name string
		loc  int64
		qty  int
	}{
		{"qty", st.location, 6},
		{"location", st.location2, 4},
	} {
		if err := st.s.Putaway(st.ctx, st.task.ID, tc.loc, tc.qty, "test", "pa-key-1"); errcode.From(err).Code != errcode.IdempotencyKeyReused.Code {
			t.Fatalf("%s: err=%v", tc.name, err)
		}
	}
	if inv := st.inventory(t); inv.StockQuantity != 4 {
		t.Fatalf("conflicts must not change stock: %d", inv.StockQuantity)
	}
	// 新 key 上架剩余 6 件：正常累计，任务完成、单据 COMPLETED
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 6, "test", "pa-key-2"); err != nil {
		t.Fatalf("second putaway: %v", err)
	}
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 6, "test", "pa-key-2"); err != nil {
		t.Fatalf("replay after completed: %v", err)
	}
	if inv := st.inventory(t); inv.StockQuantity != 10 {
		t.Fatalf("final stock=%d want=10", inv.StockQuantity)
	}
	if n := st.transCount(t); n != 2 {
		t.Fatalf("final trans=%d want=2", n)
	}
	if gotTask := st.reloadTask(t); gotTask.DoneQty != 10 || gotTask.Status != taskmodel.TaskCompleted {
		t.Fatalf("final task=%+v", gotTask)
	}
	var order model.ReceiptOrder
	if err := st.db.WithContext(st.ctx).First(&order, st.order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != model.OrderCompleted {
		t.Fatalf("order status=%s want=COMPLETED", order.Status)
	}
	if loc := st.reloadLocation(t); loc.Status != basicmodel.LocationStatusOccupied {
		t.Fatalf("location status=%d want=OCCUPIED", loc.Status)
	}
}

// TestPutawayFailureRollsBack 上架中途失败（任务推进注入错误）时，
// 库存、流水、库位状态与任务进度全部回滚，不留下半成功状态。
func TestPutawayFailureRollsBack(t *testing.T) {
	st := newPutawayIdemStack(t, 10)

	var once sync.Once
	if err := st.db.Callback().Update().Before("gorm:update").Register("pa:task_update_fail", func(q *gorm.DB) {
		if q.Statement == nil || q.Statement.Table != "wms_task" {
			return
		}
		once.Do(func() { _ = q.AddError(errors.New("injected task update failure")) })
	}); err != nil {
		t.Fatal(err)
	}

	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 4, "test", "pa-key-fail"); errcode.From(err).Code != errcode.Internal.Code {
		t.Fatalf("putaway err=%v", err)
	}
	if inv := st.inventory(t); inv.StockQuantity != 0 {
		t.Fatalf("stock after rollback=%d want=0", inv.StockQuantity)
	}
	if n := st.transCount(t); n != 0 {
		t.Fatalf("trans after rollback=%d want=0", n)
	}
	if loc := st.reloadLocation(t); loc.Status != basicmodel.LocationStatusIdle {
		t.Fatalf("location status=%d want=IDLE", loc.Status)
	}
	if gotTask := st.reloadTask(t); gotTask.DoneQty != 0 || gotTask.Status != taskmodel.TaskCreated {
		t.Fatalf("task after rollback=%+v", gotTask)
	}
}

// TestPutawayIdempotencyCorruptAndIsolation 损坏空成功标记返回内部错误；跨租户/跨 scope 同 key 隔离。
func TestPutawayIdempotencyCorruptAndIsolation(t *testing.T) {
	st := newPutawayIdemStack(t, 10)

	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 3, "test", "pa-key-corrupt"); err != nil {
		t.Fatalf("first putaway: %v", err)
	}
	for _, shape := range []string{`null`, `{"x":1}`, ``} {
		if err := st.db.WithContext(st.ctx).Model(&idempotency.Record{}).
			Where("idempotency_key = ?", "pa-key-corrupt").Update("result_json", shape).Error; err != nil {
			t.Fatal(err)
		}
		if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 3, "test", "pa-key-corrupt"); errcode.From(err).Code != errcode.Internal.Code {
			t.Fatalf("shape %q: err=%v", shape, err)
		}
	}

	// 跨租户 / 跨 scope 的同 key fixture 不影响本租户新操作
	fixtures := []*idempotency.Record{
		{TenantID: 32, Scope: "inbound.putaway", IdempotencyKey: "pa-key-iso"},
		{TenantID: 31, Scope: "other.scope", IdempotencyKey: "pa-key-iso"},
	}
	for _, fixture := range fixtures {
		fixture.ID = snowflake.Next()
		fixture.RequestHash = "other-hash"
		fixture.ObjectID = st.task.ID
		fixture.ResultJSON = idempotency.EmptySuccessJSON
		// 平台旁路上下文写入跨租户 fixture：正租户上下文不允许显式写其他 tenant_id。
		if err := st.db.WithContext(tenant.WithTenant(context.Background(), 0)).Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 3, "test", "pa-key-iso"); err != nil {
		t.Fatalf("isolated putaway: %v", err)
	}
	if gotTask := st.reloadTask(t); gotTask.DoneQty != 6 {
		t.Fatalf("task done=%d want=6", gotTask.DoneQty)
	}
}
