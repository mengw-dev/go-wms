package service

import (
	"context"
	"errors"
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
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func TestReceiveCountsDefectiveGoodsOnce(t *testing.T) {
	for _, tt := range []struct {
		name            string
		batches         []dto.ReceiveReq
		good, defective int
	}{
		{"mixed", []dto.ReceiveReq{{Qty: 10, DefectiveQty: 2}}, 8, 2},
		{"all defective", []dto.ReceiveReq{{Qty: 10, DefectiveQty: 10}}, 0, 10},
		{"partial", []dto.ReceiveReq{{Qty: 4, DefectiveQty: 1}, {Qty: 6, DefectiveQty: 1}}, 8, 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dsn := os.Getenv("WMS_TEST_DSN")
			if dsn == "" {
				dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
			}
			db := testutil.OpenIsolatedMySQL(t, dsn, &model.ReceiptOrder{}, &model.ReceiptOrderDetail{}, &taskmodel.Task{})
			if err := tenant.RegisterGORMCallbacks(db); err != nil {
				t.Fatal(err)
			}
			ctx := tenant.WithTenant(context.Background(), 11)
			repo := repository.New()
			tasks := taskservice.New(taskrepo.New(), db)
			s := &Service{repo: repo, tm: tx.New(db), taskAPI: tasks}
			order := &model.ReceiptOrder{OrderNo: "RECEIVE-TEST", Status: model.OrderApproved, WarehouseID: 1, ExpectedQty: 10}
			detail := &model.ReceiptOrderDetail{SKUID: 1, ExpectedQty: 10}
			if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := repo.CreateOrder(tx, order, []*model.ReceiptOrderDetail{detail}); err != nil {
					return err
				}
				return tasks.Create(ctx, tx, []*taskapi.CreateTask{{TaskType: taskmodel.TaskReceive, OrderID: order.ID, OrderNo: order.OrderNo, DetailID: detail.ID, SKUID: 1, WarehouseID: 1, TargetQty: 10}})
			}); err != nil {
				t.Fatal(err)
			}
			// 越界请求不能改变主单、任务或明细。
			if err := s.Receive(ctx, order.ID, detail.ID, &dto.ReceiveReq{Qty: 11, BatchNo: "B"}, "test"); !errors.Is(err, errcode.ReceiveQtyOver) {
				t.Fatalf("over receive: %v", err)
			}
			for _, batch := range tt.batches {
				batch.BatchNo = "B"
				if err := s.Receive(ctx, order.ID, detail.ID, &batch, "test"); err != nil {
					t.Fatal(err)
				}
			}
			got, err := repo.GetOrder(ctx, db, order.ID)
			if err != nil {
				t.Fatal(err)
			}
			wantStatus := model.OrderPutaway
			if tt.good == 0 {
				wantStatus = model.OrderCompleted
			}
			if got.ReceivedQty != 10 || got.DefectiveQty != tt.defective || got.Status != wantStatus {
				t.Fatalf("order=%+v", got)
			}
			var all []*taskmodel.Task
			if err := db.WithContext(ctx).Where("order_id = ?", order.ID).Find(&all).Error; err != nil {
				t.Fatal(err)
			}
			putawayQty := 0
			for _, task := range all {
				if task.TaskType == taskmodel.TaskReceive && (task.DoneQty != 10 || task.Status != taskmodel.TaskCompleted) {
					t.Fatalf("receive task=%+v", task)
				}
				if task.TaskType == taskmodel.TaskPutaway {
					putawayQty += task.TargetQty
				}
			}
			if putawayQty != tt.good {
				t.Fatalf("putaway quantity=%d want=%d", putawayQty, tt.good)
			}
		})
	}
}

func TestReceiveRejectsInvalidQuantitiesBeforeTransaction(t *testing.T) {
	s := &Service{}
	for _, req := range []dto.ReceiveReq{{Qty: 0}, {Qty: -1}, {Qty: 1, DefectiveQty: -1}, {Qty: 1, DefectiveQty: 2}} {
		if err := s.Receive(context.Background(), 1, 1, &req, "test"); !errors.Is(err, errcode.ParamError) {
			t.Fatalf("req=%+v err=%v", req, err)
		}
	}
}
