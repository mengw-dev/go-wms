package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/task/api"
	"gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func TestTaskAPIHonorsContextInsidePlatformTransaction(t *testing.T) {
	s, db, owner := taskFixture(t)
	platform := context.Background()
	if err := db.Transaction(func(tx *gorm.DB) error {
		return s.Create(owner, tx, []*api.CreateTask{{TaskType: model.TaskPick, TargetQty: 10, OrderID: 1, DetailID: 1, WarehouseID: 1, SKUID: 1}})
	}); err != nil {
		t.Fatal(err)
	}
	var task model.Task
	if err := db.First(&task).Error; err != nil {
		t.Fatal(err)
	}
	if task.TenantID != 99 {
		t.Fatalf("created task belongs to %d", task.TenantID)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if _, err := s.GetForUpdate(platform, tx, task.ID); !errors.Is(err, errcode.TaskNotFound) {
			t.Fatalf("platform lock: %v", err)
		}
		if err := s.AddProgress(platform, tx, &task, 1, "foreign"); !errors.Is(err, errcode.TaskNotFound) {
			t.Fatalf("platform progress: %v", err)
		}
		if task.DoneQty != 0 || task.Status != model.TaskCreated {
			t.Fatalf("foreign task snapshot mutated: %+v", task)
		}
		if err := s.AddProgressByDetail(platform, tx, 1, 1, model.TaskPick, 1, "foreign"); !errors.Is(err, errcode.TaskNotFound) {
			t.Fatalf("detail progress: %v", err)
		}
		if rows, err := s.ListByOrderForUpdate(platform, tx, 1); err != nil || len(rows) != 0 {
			t.Fatalf("foreign lock list: %v %v", rows, err)
		}
		if n, err := s.CountUnfinished(platform, tx, 1, model.TaskPick); err != nil || n != 0 {
			t.Fatalf("foreign unfinished: %d %v", n, err)
		}
		if rows, err := s.ListForWave(platform, tx, []int64{1}, 10, true); err != nil || len(rows) != 0 {
			t.Fatalf("foreign wave tasks: %v %v", rows, err)
		}
		if n, err := s.Claim(platform, tx, task.ID, "foreign", "bad", time.Now().Add(time.Hour)); err != nil || n != 0 {
			t.Fatalf("foreign claim: %d %v", n, err)
		}
		if n, err := s.Claim(owner, tx, task.ID, "owner", "good", time.Now().Add(time.Hour)); err != nil || n != 1 {
			t.Fatalf("owner claim: %d %v", n, err)
		}
		if n, err := s.RenewClaim(platform, tx, task.ID, "good", time.Now().Add(2*time.Hour)); err != nil || n != 0 {
			t.Fatalf("foreign renewal: %d %v", n, err)
		}
		if err := s.CancelByOrder(platform, tx, 1); err != nil {
			return err
		}
		locked, err := s.GetForUpdate(owner, tx, task.ID)
		if err != nil {
			return err
		}
		if locked.Status != model.TaskCreated {
			t.Fatalf("foreign cancel changed task: %+v", locked)
		}
		return s.AddProgress(owner, tx, locked, 2, "owner")
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&task, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if task.DoneQty != 2 || task.Operator != "owner" {
		t.Fatalf("owner progress: %+v", task)
	}
}
