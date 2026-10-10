package service

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func receiveTestDSN() string {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	return dsn
}

// receiveIdemStack 与 app.New 相同的收货链路最小组装（含幂等表）。
type receiveIdemStack struct {
	db     *gorm.DB
	s      *Service
	ctx    context.Context
	order  *model.ReceiptOrder
	detail *model.ReceiptOrderDetail
}

func newReceiveIdemStack(t *testing.T, expectedQty int) *receiveIdemStack {
	t.Helper()
	db := testutil.OpenIsolatedMySQL(t, receiveTestDSN(), &model.ReceiptOrder{}, &model.ReceiptOrderDetail{}, &taskmodel.Task{}, &idempotency.Record{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithTenant(context.Background(), 21)
	repo := repository.New()
	tasks := taskservice.New(taskrepo.New(), db)
	s := &Service{repo: repo, tm: tx.New(db), taskAPI: tasks}
	order := &model.ReceiptOrder{OrderNo: "IDEM-RECEIVE", Status: model.OrderApproved, WarehouseID: 1, ExpectedQty: expectedQty}
	detail := &model.ReceiptOrderDetail{SKUID: 1, ExpectedQty: expectedQty}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repo.CreateOrder(tx, order, []*model.ReceiptOrderDetail{detail}); err != nil {
			return err
		}
		return tasks.Create(ctx, tx, []*taskapi.CreateTask{{TaskType: taskmodel.TaskReceive, OrderID: order.ID, OrderNo: order.OrderNo, DetailID: detail.ID, SKUID: 1, WarehouseID: 1, TargetQty: expectedQty}})
	}); err != nil {
		t.Fatal(err)
	}
	return &receiveIdemStack{db: db, s: s, ctx: ctx, order: order, detail: detail}
}

func (st *receiveIdemStack) reload(t *testing.T) (*model.ReceiptOrder, *model.ReceiptOrderDetail) {
	t.Helper()
	var order model.ReceiptOrder
	if err := st.db.WithContext(st.ctx).First(&order, st.order.ID).Error; err != nil {
		t.Fatal(err)
	}
	var detail model.ReceiptOrderDetail
	if err := st.db.WithContext(st.ctx).First(&detail, st.detail.ID).Error; err != nil {
		t.Fatal(err)
	}
	return &order, &detail
}

func (st *receiveIdemStack) putawayTasks(t *testing.T) []*taskmodel.Task {
	t.Helper()
	var all []*taskmodel.Task
	if err := st.db.WithContext(st.ctx).Where("order_id = ? AND task_type = ?", st.order.ID, taskmodel.TaskPutaway).Find(&all).Error; err != nil {
		t.Fatal(err)
	}
	return all
}

// TestReceiveIdempotencyReplay 收货请求级幂等：
// 同 key 重试回放空成功不重复累计；新 key 同参数是另一次合法收货；
// 收齐后的重放不重复生成上架任务；残品只累计一次。
func TestReceiveIdempotencyReplay(t *testing.T) {
	st := newReceiveIdemStack(t, 100)
	req := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 30, DefectiveQty: 5, BatchNo: "BATCH-A"}

	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req, "test", "recv-key-1"); err != nil {
		t.Fatalf("first receive: %v", err)
	}
	// 同 key 同内容重试：回放空成功，不再次累计
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req, "test", "recv-key-1"); err != nil {
		t.Fatalf("replay receive: %v", err)
	}
	order, detail := st.reload(t)
	if order.ReceivedQty != 30 || order.DefectiveQty != 5 || detail.ReceivedQty != 30 {
		t.Fatalf("after replay: order=%+v detail=%+v", order, detail)
	}
	// 新 key 再收 30：正常累计到 60
	req2 := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 30, BatchNo: "BATCH-A"}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req2, "test", "recv-key-2"); err != nil {
		t.Fatalf("second receive: %v", err)
	}
	order, _ = st.reload(t)
	if order.ReceivedQty != 60 {
		t.Fatalf("after second receive: received=%d want=60", order.ReceivedQty)
	}
	// 同 key 改变内容（数量/批次/明细）→ 409 内容冲突
	for _, tc := range []struct {
		name string
		key  string
		req  dto.ReceiveReq
	}{
		{"qty", "recv-key-1", dto.ReceiveReq{DetailID: st.detail.ID, Qty: 40, BatchNo: "BATCH-A"}},
		{"batch", "recv-key-2", dto.ReceiveReq{DetailID: st.detail.ID, Qty: 30, BatchNo: "BATCH-B"}},
		{"detail", "recv-key-1", dto.ReceiveReq{DetailID: st.detail.ID + 999, Qty: 30, BatchNo: "BATCH-A"}},
	} {
		if err := st.s.Receive(st.ctx, st.order.ID, tc.req.DetailID, &tc.req, "test", tc.key); errcode.From(err).Code != errcode.IdempotencyKeyReused.Code {
			t.Fatalf("%s: err=%v", tc.name, err)
		}
	}
	order, _ = st.reload(t)
	if order.ReceivedQty != 60 {
		t.Fatalf("conflicts must not change data: received=%d", order.ReceivedQty)
	}
	// 收齐（收 40）：生成上架任务；同 key 重放不重复生成
	req3 := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 40, BatchNo: "BATCH-A"}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req3, "test", "recv-key-3"); err != nil {
		t.Fatalf("final receive: %v", err)
	}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req3, "test", "recv-key-3"); err != nil {
		t.Fatalf("replay after fully received: %v", err)
	}
	order, _ = st.reload(t)
	if order.Status != model.OrderPutaway || order.ReceivedQty != 100 {
		t.Fatalf("final order=%+v", order)
	}
	tasks := st.putawayTasks(t)
	if len(tasks) != 1 || tasks[0].TargetQty != 95 {
		t.Fatalf("putaway tasks=%+v", tasks)
	}
}

// TestReceiveIdempotencyBusinessFailure 业务失败时不留下成功记录，key 可复用执行。
func TestReceiveIdempotencyBusinessFailure(t *testing.T) {
	st := newReceiveIdemStack(t, 10)
	// 超量收货：明确拒绝且不留记录
	req := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 11, BatchNo: "B1"}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req, "test", "recv-key-fail"); errcode.From(err).Code != errcode.ReceiveQtyOver.Code {
		t.Fatalf("over receive: %v", err)
	}
	// 同一 key 携带正确数量：作为新操作执行成功（此前失败已回滚，不形成记录）
	ok := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 10, BatchNo: "B1"}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &ok, "test", "recv-key-fail"); err != nil {
		t.Fatalf("reuse key after rollback: %v", err)
	}
}

// TestReceiveIdempotencyCorruptResult 空成功标记损坏（非空对象/null/非法 JSON）时重试返回内部错误。
func TestReceiveIdempotencyCorruptResult(t *testing.T) {
	st := newReceiveIdemStack(t, 10)
	req := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 5, BatchNo: "B1"}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req, "test", "recv-key-corrupt"); err != nil {
		t.Fatalf("first receive: %v", err)
	}
	for _, shape := range []string{`null`, ``, `{"x":1}`} {
		if err := st.db.WithContext(st.ctx).Model(&idempotency.Record{}).
			Where("idempotency_key = ?", "recv-key-corrupt").Update("result_json", shape).Error; err != nil {
			t.Fatal(err)
		}
		if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req, "test", "recv-key-corrupt"); errcode.From(err).Code != errcode.Internal.Code {
			t.Fatalf("shape %q: err=%v", shape, err)
		}
	}
}

// TestReceiveIdempotencyIsolation 幂等键按 (tenant_id, scope) 隔离：其他租户/其他 scope 的同 key 不影响本租户新操作。
func TestReceiveIdempotencyIsolation(t *testing.T) {
	st := newReceiveIdemStack(t, 10)
	fixtures := []*idempotency.Record{
		{TenantID: 22, Scope: "inbound.receive", IdempotencyKey: "recv-iso"},
		{TenantID: 21, Scope: "other.scope", IdempotencyKey: "recv-iso"},
	}
	for _, fixture := range fixtures {
		fixture.ID = snowflake.Next()
		fixture.RequestHash = "other-hash"
		fixture.ObjectID = st.order.ID
		fixture.ResultJSON = idempotency.EmptySuccessJSON
		// 平台旁路上下文写入跨租户 fixture：正租户上下文不允许显式写其他 tenant_id。
		if err := st.db.WithContext(tenant.WithTenant(context.Background(), 0)).Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}
	req := dto.ReceiveReq{DetailID: st.detail.ID, Qty: 5, BatchNo: "B1"}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &req, "test", "recv-iso"); err != nil {
		t.Fatalf("isolated receive: %v", err)
	}
	_, detail := st.reload(t)
	if detail.ReceivedQty != 5 {
		t.Fatalf("detail received=%d want=5", detail.ReceivedQty)
	}
}
