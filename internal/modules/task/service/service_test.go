package service

import (
	"context"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/task/api"
	"gowms/internal/modules/task/model"
	"gowms/internal/modules/task/repository"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func taskFixture(t *testing.T) (*Service, *gorm.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.Task{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	return New(repository.New(), db), db, tenant.WithTenant(context.Background(), 99)
}

func TestTaskCreationSharesCallerTransaction(t *testing.T) {
	s, db, ctx := taskFixture(t)
	creates := []*api.CreateTask{
		{TaskType: model.TaskReceive, TargetQty: 10, OrderID: 1, WarehouseID: 1, SKUID: 1},
		{TaskType: model.TaskPutaway, TargetQty: 10, OrderID: 1, WarehouseID: 1, SKUID: 1},
		{TaskType: model.TaskPick, TargetQty: 10, OrderID: 2, WarehouseID: 1, SKUID: 1},
	}
	abort := errors.New("later business operation failed")
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.Create(ctx, tx, creates); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.Task{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tasks survived rollback: %d", count)
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.Create(ctx, tx, creates) }); err != nil {
		t.Fatal(err)
	}
	var tasks []*model.Task
	if err := db.Order("id").Find(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 3 {
		t.Fatalf("created=%d", len(tasks))
	}
	for i, task := range tasks {
		if task.TenantID != 99 || !strings.HasPrefix(task.TaskNo, []string{"SH", "SJ", "PK"}[i]) || !strings.HasSuffix(task.TaskNo, strconv.FormatInt(task.ID, 10)) || len(task.TaskNo) > 64 {
			t.Fatalf("unexpected task: %+v", task)
		}
	}
}

func TestTaskConcurrentProgressDoesNotOverComplete(t *testing.T) {
	s, db, ctx := taskFixture(t)
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.Create(ctx, tx, []*api.CreateTask{{TaskType: model.TaskReceive, TargetQty: 10, OrderID: 1, WarehouseID: 1, SKUID: 1}})
	}); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err := db.First(&task).Error; err != nil {
		t.Fatal(err)
	}
	progress := func(qty int) error {
		return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { return s.AddProgress(ctx, tx, task.ID, qty, "test") })
	}
	if err := progress(1); err != nil {
		t.Fatal(err)
	}
	if err := progress(math.MaxInt); !errors.Is(err, errcode.TaskQtyOver) {
		t.Fatalf("overflow quantity: %v", err)
	}
	var success, rejected atomic.Int64
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			err := progress(1)
			switch {
			case err == nil:
				success.Add(1)
			case errors.Is(err, errcode.TaskStatusWrong):
				rejected.Add(1)
			default:
				t.Errorf("unexpected progress error: %v", err)
			}
		})
	}
	wg.Wait()
	if success.Load() != 9 || rejected.Load() != 11 {
		t.Fatalf("success=%d rejected=%d", success.Load(), rejected.Load())
	}
	got, err := s.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DoneQty != 10 || got.Status != model.TaskCompleted {
		t.Fatalf("task=%+v", got)
	}
	otherTenant := tenant.WithTenant(ctx, 100)
	if _, err := s.Get(otherTenant, task.ID); !errors.Is(err, errcode.TaskNotFound) {
		t.Fatalf("cross-tenant query: %v", err)
	}
}

func TestCreateRejectsInvalidTasks(t *testing.T) {
	s := &Service{}
	for _, task := range []*api.CreateTask{nil, {TaskType: model.TaskReceive, TargetQty: 0}, {TaskType: "UNKNOWN", TargetQty: 1}} {
		if err := s.Create(context.Background(), nil, []*api.CreateTask{task}); !errors.Is(err, errcode.ParamError) {
			t.Fatalf("task=%+v err=%v", task, err)
		}
	}
}

func TestCancelByOrderInvalidatesStaleProgress(t *testing.T) {
	s, db, ctx := taskFixture(t)
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.Create(ctx, tx, []*api.CreateTask{{TaskType: model.TaskPick, TargetQty: 10, OrderID: 1, WarehouseID: 1, SKUID: 1}})
	}); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err := db.WithContext(ctx).First(&task).Error; err != nil {
		t.Fatal(err)
	}
	stale := task
	stale.DoneQty = task.TargetQty
	stale.Status = model.TaskCompleted

	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.CancelByOrder(ctx, tx, task.OrderID)
	}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.repo.UpdateProgress(db.WithContext(ctx), &stale); err != nil || n != 0 {
		t.Fatalf("stale progress update rows=%d err=%v", n, err)
	}
	got, err := s.Get(ctx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.TaskCancelled || got.Version != task.Version+1 {
		t.Fatalf("cancelled task=%+v, original=%+v", got, task)
	}
}
