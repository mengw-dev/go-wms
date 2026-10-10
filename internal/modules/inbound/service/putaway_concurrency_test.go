package service

import (
	"context"
	"fmt"
	"testing"
	"time"

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
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

// 并发上架回归（P1）：不同 key 并发完成同一单据的不同上架任务，
// 后执行事务的完成统计必须看到先执行事务的任务完成，单据进入 COMPLETED。

// putawayConcurrentStack 双任务上架链路最小组装（与 app.New 一致，含幂等表）。
type putawayConcurrentStack struct {
	db      *gorm.DB
	s       *Service
	ctx     context.Context
	order   *model.ReceiptOrder
	tasks   []*taskmodel.Task
	loc1    int64
	loc2    int64
	perTask int
}

func newPutawayConcurrentStack(t *testing.T, perTask int) *putawayConcurrentStack {
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
	repo := repository.New()
	s := &Service{repo: repo, tm: tm, no: orderno.New(nil), basic: basicSvc, inv: invSvc, taskAPI: taskSvc}

	warehouseID := snowflake.Next()
	skuID := snowflake.Next()
	loc1 := snowflake.Next()
	loc2 := snowflake.Next()
	if err := db.WithContext(ctx).Create(&basicmodel.Warehouse{
		Base: modelbase.Base{ID: warehouseID}, Code: "PACS-WH", Name: "putaway conc", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&basicmodel.SKU{
		Base: modelbase.Base{ID: skuID}, Code: "PACS-SKU", Barcode: "PACS-BC", Name: "putaway conc", Unit: "件", Status: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, loc := range []*basicmodel.Location{
		{Base: modelbase.Base{ID: loc1}, WarehouseID: warehouseID, Code: "PACS-L1", Status: basicmodel.LocationStatusIdle},
		{Base: modelbase.Base{ID: loc2}, WarehouseID: warehouseID, Code: "PACS-L2", Status: basicmodel.LocationStatusIdle},
	} {
		if err := db.WithContext(ctx).Create(loc).Error; err != nil {
			t.Fatal(err)
		}
	}

	order := &model.ReceiptOrder{OrderNo: "PACS-ORDER", Status: model.OrderPutaway, WarehouseID: warehouseID, ExpectedQty: perTask * 2}
	details := []*model.ReceiptOrderDetail{
		{SKUID: skuID, ExpectedQty: perTask, BatchNo: "B1"},
		{SKUID: skuID, ExpectedQty: perTask, BatchNo: "B1"},
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repo.CreateOrder(tx, order, details); err != nil {
			return err
		}
		creates := make([]*taskapi.CreateTask, 0, len(details))
		for _, detail := range details {
			creates = append(creates, &taskapi.CreateTask{
				TaskType: taskmodel.TaskPutaway, OrderID: order.ID, OrderNo: order.OrderNo,
				DetailID: detail.ID, SKUID: detail.SKUID, WarehouseID: warehouseID, TargetQty: detail.ExpectedQty,
			})
		}
		return taskSvc.Create(ctx, tx, creates)
	}); err != nil {
		t.Fatal(err)
	}
	var taskList []*taskmodel.Task
	if err := db.WithContext(ctx).Where("order_id = ? AND task_type = ?", order.ID, taskmodel.TaskPutaway).
		Order("id").Find(&taskList).Error; err != nil {
		t.Fatal(err)
	}
	if len(taskList) != 2 {
		t.Fatalf("fixture tasks=%d want=2", len(taskList))
	}
	return &putawayConcurrentStack{db: db, s: s, ctx: ctx, order: order, tasks: taskList, loc1: loc1, loc2: loc2, perTask: perTask}
}

// TestPutawayConcurrentTasksCompletesOrder 并发上架同一单据的两个任务：
// 两个请求都成功后单据必须 COMPLETED。回归保护：事务内幂等查询曾提前固定
// REPEATABLE READ 读视图，后执行事务的 CountUnfinished 漏看先执行事务的任务完成，
// 复现「未完成任务已是 0、单据仍停在 PUTAWAY」。
func TestPutawayConcurrentTasksCompletesOrder(t *testing.T) {
	st := newPutawayConcurrentStack(t, 5)
	// 评审复现环境：小连接池下并发事务不得因「事务内再借第二条连接」互相等待。
	if sqlDB, err := st.db.DB(); err != nil {
		t.Fatal(err)
	} else {
		sqlDB.SetMaxOpenConns(2)
	}
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	if err := st.db.Callback().Query().After("gorm:query").Register("pa:concurrent_idem", func(q *gorm.DB) {
		if q.Statement == nil || q.Statement.Table != "wms_idempotency" {
			return
		}
		arrived <- struct{}{}
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 2)
	for i, task := range st.tasks {
		go func(taskID int64, loc int64, key string) {
			errCh <- st.s.Putaway(st.ctx, taskID, loc, st.perTask, "test", key)
		}(task.ID, []int64{st.loc1, st.loc2}[i], fmt.Sprintf("pa-conc-%d", i))
	}
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("both putaways did not reach idempotency query")
		}
	}
	close(release)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("putaway: %v", err)
		}
	}
	var order model.ReceiptOrder
	if err := st.db.WithContext(st.ctx).First(&order, st.order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if order.Status != model.OrderCompleted {
		t.Fatalf("order status=%s want=COMPLETED", order.Status)
	}
	for _, task := range st.tasks {
		var reloaded taskmodel.Task
		if err := st.db.WithContext(st.ctx).First(&reloaded, task.ID).Error; err != nil {
			t.Fatal(err)
		}
		if reloaded.Status != taskmodel.TaskCompleted || reloaded.DoneQty != st.perTask {
			t.Fatalf("task=%+v", reloaded)
		}
	}
}
