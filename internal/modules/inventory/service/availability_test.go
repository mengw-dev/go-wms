package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/errcode"
)

func TestInsufficientStockReportsPreallocationAvailability(t *testing.T) {
	for _, batches := range [][]int{{10}, {4, 6}, {0}} {
		t.Run(fmt.Sprint(batches), func(t *testing.T) {
			s, tm, db := newTestService(t)
			total := 0
			for i, qty := range batches {
				total += qty
				if err := db.Create(&model.Inventory{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: fmt.Sprint(i), StockQuantity: qty, AvailableQty: qty, StockInTime: time.Now()}).Error; err != nil {
					t.Fatal(err)
				}
			}
			err := tm.Tx(context.Background(), func(q *gorm.DB) error {
				_, err := s.Allocate(context.Background(), q, &api.AllocateReq{WarehouseID: 1, SKUID: 1, Quantity: 15})
				return err
			})
			if errcode.From(err).Code != errcode.AvailableNotEnough.Code || errcode.From(err).Msg != availableNotEnoughMsg(1, 15, total) {
				t.Fatalf("wrong availability: %v", err)
			}
			var rows []model.Inventory
			if err := db.Find(&rows).Error; err != nil {
				t.Fatal(err)
			}
			available := 0
			for _, row := range rows {
				available += row.AvailableQty
				if row.AllocatedQty != 0 {
					t.Fatal("allocation survived rollback")
				}
			}
			var count int64
			if err := db.Model(&model.InventoryTrans{}).Count(&count).Error; err != nil {
				t.Fatal(err)
			}
			if available != total || count != 0 {
				t.Fatalf("rollback incomplete: available=%d trans=%d", available, count)
			}
		})
	}
}

func TestAllocationSnapshotChangeStillReturnsConflict(t *testing.T) {
	s, tm, db := newTestService(t)
	row := &model.Inventory{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B", StockQuantity: 10, AvailableQty: 10, StockInTime: time.Now()}
	if err := db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	if err := db.Callback().Query().Before("gorm:query").Register("test:change_before_lock", func(q *gorm.DB) {
		if q.Statement.Table != "wms_inventory" {
			return
		}
		if _, locking := q.Statement.Clauses["FOR"]; !locking {
			return
		}
		once.Do(func() {
			// 独立连接在候选快照之后提交变更；被测事务随后锁读到最新值。
			if err := db.Model(&model.Inventory{}).Where("id = ?", row.ID).Updates(map[string]any{"stock_quantity": 5, "available_quantity": 5}).Error; err != nil {
				q.Error = errors.Join(q.Error, err)
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	err := tm.Tx(context.Background(), func(q *gorm.DB) error {
		_, err := s.Allocate(context.Background(), q, &api.AllocateReq{WarehouseID: 1, SKUID: 1, Quantity: 15})
		return err
	})
	if !errors.Is(err, errcode.Conflict) {
		t.Fatalf("stale candidate must retry instead of reporting a misleading shortage: %v", err)
	}
	var current model.Inventory
	if err := db.First(&current, row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.AvailableQty != 5 || current.AllocatedQty != 0 {
		t.Fatalf("partial allocation survived: %+v", current)
	}
}
