package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	taskapi "gowms/internal/modules/task/api"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

// 并发收货回归（P1）：不同 key 并发收同一单据的不同明细，收齐判断必须基于最新收货量。

// receiveConcurrentStack 双明细收货链路最小组装（与 app.New 一致，含幂等表）。
type receiveConcurrentStack struct {
	db      *gorm.DB
	s       *Service
	ctx     context.Context
	order   *model.ReceiptOrder
	details []*model.ReceiptOrderDetail
}

func newReceiveConcurrentStack(t *testing.T) *receiveConcurrentStack {
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
	order := &model.ReceiptOrder{OrderNo: "CONC-RECEIVE", Status: model.OrderApproved, WarehouseID: 1, ExpectedQty: 2}
	details := []*model.ReceiptOrderDetail{
		{SKUID: 1, ExpectedQty: 1},
		{SKUID: 2, ExpectedQty: 1},
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repo.CreateOrder(tx, order, details); err != nil {
			return err
		}
		creates := make([]*taskapi.CreateTask, 0, len(details))
		for _, detail := range details {
			creates = append(creates, &taskapi.CreateTask{
				TaskType: taskmodel.TaskReceive, OrderID: order.ID, OrderNo: order.OrderNo,
				DetailID: detail.ID, SKUID: detail.SKUID, WarehouseID: 1, TargetQty: detail.ExpectedQty,
			})
		}
		return tasks.Create(ctx, tx, creates)
	}); err != nil {
		t.Fatal(err)
	}
	return &receiveConcurrentStack{db: db, s: s, ctx: ctx, order: order, details: details}
}

// TestReceiveConcurrentDifferentDetails 同单不同明细并发收货，两个请求都成功提交后：
// 单据必须进入 PUTAWAY 并生成上架任务。回归保护：事务内幂等查询曾提前固定
// REPEATABLE READ 读视图，后执行事务的收齐判断漏看另一明细已提交的收货量，
// 复现「应收 2 已收 2 仍 RECEIVING、上架任务 0」。
func TestReceiveConcurrentDifferentDetails(t *testing.T) {
	st := newReceiveConcurrentStack(t)
	// 评审复现环境：小连接池下并发事务不得因「事务内再借第二条连接」互相等待。
	if sqlDB, err := st.db.DB(); err != nil {
		t.Fatal(err)
	} else {
		sqlDB.SetMaxOpenConns(2)
	}
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	if err := st.db.Callback().Query().After("gorm:query").Register("recv:concurrent_idem", func(q *gorm.DB) {
		if q.Statement == nil || q.Statement.Table != "wms_idempotency" {
			return
		}
		arrived <- struct{}{}
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 2)
	for i, detail := range st.details {
		go func(detailID int64, key string) {
			errCh <- st.s.Receive(st.ctx, st.order.ID, detailID,
				&dto.ReceiveReq{DetailID: detailID, Qty: 1, BatchNo: "B1"}, "test", key)
		}(detail.ID, fmt.Sprintf("recv-conc-%d", i))
	}
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("both requests did not reach idempotency query")
		}
	}
	close(release)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("receive: %v", err)
		}
	}
	var order model.ReceiptOrder
	if err := st.db.WithContext(st.ctx).First(&order, st.order.ID).Error; err != nil {
		t.Fatal(err)
	}
	if order.ReceivedQty != 2 || order.Status != model.OrderPutaway {
		t.Fatalf("order=%+v want received=2 status=PUTAWAY", order)
	}
	var putaway []*taskmodel.Task
	if err := st.db.WithContext(st.ctx).Where("order_id = ? AND task_type = ?", order.ID, taskmodel.TaskPutaway).Find(&putaway).Error; err != nil {
		t.Fatal(err)
	}
	if len(putaway) != 2 {
		t.Fatalf("putaway tasks=%d want=2", len(putaway))
	}
}

// TestReceiveWithKeyUnderSmallPool 连接池上限 1：带 key 收货及其同 key 重试都必须完成。
// 幂等预查询若放在事务回调内，事务占住唯一连接后还要申请第二条连接，请求会一直等待。
func TestReceiveWithKeyUnderSmallPool(t *testing.T) {
	st := newReceiveConcurrentStack(t)
	if sqlDB, err := st.db.DB(); err != nil {
		t.Fatal(err)
	} else {
		sqlDB.SetMaxOpenConns(1)
	}
	detail := st.details[0]
	req := &dto.ReceiveReq{DetailID: detail.ID, Qty: 1, BatchNo: "B1"}
	if err := st.s.Receive(st.ctx, st.order.ID, detail.ID, req, "test", "recv-small-pool"); err != nil {
		t.Fatalf("receive with key under pool=1: %v", err)
	}
	if err := st.s.Receive(st.ctx, st.order.ID, detail.ID, req, "test", "recv-small-pool"); err != nil {
		t.Fatalf("replay under pool=1: %v", err)
	}
}
