package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/dto"
	basicmodel "gowms/internal/modules/basic/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
)

// 并发创建同一货品时只能有一个成功，其余必须得到明确的业务错误，而不是数据库内部错误。
func TestCreateSKUConcurrentDuplicateReturnsBusinessError(t *testing.T) {
	s, _, _, _ := newDeleteTestService(t)
	ctx := context.Background()

	const workers = 4
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- s.CreateSKU(ctx, &dto.SKUReq{Code: "SKU-RACE", Barcode: "BAR-RACE", Name: "并发货品"})
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	success, duplicated := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, errcode.SKUExist):
			duplicated++
		default:
			t.Fatalf("unexpected error (want nil or SKUExist): %v", err)
		}
	}
	if success != 1 || duplicated != workers-1 {
		t.Fatalf("success=%d duplicated=%d want 1/%d", success, duplicated, workers-1)
	}
}

// 并发窗口内两个请求都可能通过前置检查：唯一索引兜底必须回查并返回业务错误。
// 用未提交事务先占住唯一索引，待 Service 开始插入后再提交，确定性地走到冲突路径。
func TestCreateSKUConflictMapsToBusinessError(t *testing.T) {
	cases := []struct {
		name     string
		req      dto.SKUReq
		occupied basicmodel.SKU
		want     error
	}{
		{
			name:     "code",
			req:      dto.SKUReq{Code: "SKU-LOCKED", Barcode: "BAR-FREE", Name: "n"},
			occupied: basicmodel.SKU{Code: "SKU-LOCKED", Barcode: "BAR-OTHER", Name: "occupied"},
			want:     errcode.SKUExist,
		},
		{
			name:     "barcode",
			req:      dto.SKUReq{Code: "SKU-FREE", Barcode: "BAR-LOCKED", Name: "n"},
			occupied: basicmodel.SKU{Code: "SKU-OTHER", Barcode: "BAR-LOCKED", Name: "occupied"},
			want:     errcode.BarcodeExist,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _, _, db := newDeleteTestService(t)
			ctx := context.Background()

			blocker := db.Begin()
			if blocker.Error != nil {
				t.Fatal(blocker.Error)
			}
			defer func() { _ = blocker.Rollback() }()
			occupied := tc.occupied
			occupied.Base = sysmodel.Base{ID: snowflake.Next()}
			occupied.Status = 1
			if err := blocker.Create(&occupied).Error; err != nil {
				t.Fatal(err)
			}

			creating := make(chan struct{}, 1)
			if err := db.Callback().Create().Before("gorm:create").Register("test:sku-create-watch", func(db *gorm.DB) {
				if db.Statement.Table == "wms_sku" {
					select {
					case creating <- struct{}{}:
					default:
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Callback().Create().Remove("test:sku-create-watch") })

			req := tc.req
			done := make(chan error, 1)
			go func() { done <- s.CreateSKU(ctx, &req) }()
			select {
			case <-creating:
			case <-time.After(5 * time.Second):
				t.Fatal("create did not reach the insert")
			}
			if err := blocker.Commit().Error; err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, tc.want) {
				t.Fatalf("conflict on %s: got %v, want %v", tc.name, err, tc.want)
			}
		})
	}
}

// 并发创建同一仓库编码同样必须落到业务错误而不是内部错误。
func TestCreateWarehouseConcurrentDuplicateReturnsBusinessError(t *testing.T) {
	s, _, _, _ := newDeleteTestService(t)
	ctx := context.Background()

	const workers = 2
	start := make(chan struct{})
	results := make(chan error, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- s.CreateWarehouse(ctx, &dto.WarehouseReq{Code: "WH-RACE", Name: "并发仓库"})
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	success, duplicated := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, errcode.WarehouseExist):
			duplicated++
		default:
			t.Fatalf("unexpected error (want nil or WarehouseExist): %v", err)
		}
	}
	if success != 1 || duplicated != 1 {
		t.Fatalf("success=%d duplicated=%d want 1/1", success, duplicated)
	}
}

// 更新货品时编码/条码撞唯一索引也必须映射为业务错误，失败不影响原数据，更新自身允许。
func TestUpdateSKUConflictMapsToBusinessError(t *testing.T) {
	s, _, _, db := newDeleteTestService(t)
	ctx := context.Background()

	a := &basicmodel.SKU{Base: sysmodel.Base{ID: snowflake.Next()}, Code: "UPD-A", Barcode: "UPD-BAR-A", Name: "a", Status: 1}
	b := &basicmodel.SKU{Base: sysmodel.Base{ID: snowflake.Next()}, Code: "UPD-B", Barcode: "UPD-BAR-B", Name: "b", Status: 1}
	for _, sku := range []*basicmodel.SKU{a, b} {
		if err := db.Create(sku).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := s.UpdateSKU(ctx, b.ID, &dto.SKUReq{Code: "UPD-A", Barcode: "UPD-BAR-B", Name: "b"}); !errors.Is(err, errcode.SKUExist) {
		t.Fatalf("code conflict: got %v, want %v", err, errcode.SKUExist)
	}
	if err := s.UpdateSKU(ctx, b.ID, &dto.SKUReq{Code: "UPD-B", Barcode: "UPD-BAR-A", Name: "b"}); !errors.Is(err, errcode.BarcodeExist) {
		t.Fatalf("barcode conflict: got %v, want %v", err, errcode.BarcodeExist)
	}
	if unchanged, err := s.repo.GetSKU(ctx, db, b.ID); err != nil {
		t.Fatal(err)
	} else if unchanged.Code != "UPD-B" || unchanged.Barcode != "UPD-BAR-B" {
		t.Fatalf("conflict changed the sku: %+v", unchanged)
	}
	if err := s.UpdateSKU(ctx, b.ID, &dto.SKUReq{Code: "UPD-B", Barcode: "UPD-BAR-B", Name: "b2"}); err != nil {
		t.Fatalf("update self: %v", err)
	}
}
