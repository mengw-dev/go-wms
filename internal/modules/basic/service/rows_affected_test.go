package service

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/dto"
	basicmodel "gowms/internal/modules/basic/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
)

// 更新不存在的对象必须返回“不存在”业务错误，而不是把 0 行影响当成成功。
func TestUpdateMissingObjectsReturnNotFound(t *testing.T) {
	s, _, _, _ := newDeleteTestService(t)
	ctx := context.Background()

	if err := s.UpdateWarehouse(ctx, 999999, &dto.WarehouseReq{Code: "WH", Name: "n"}); !errors.Is(err, errcode.WarehouseNotFound) {
		t.Fatalf("update missing warehouse: got %v, want %v", err, errcode.WarehouseNotFound)
	}
	if err := s.UpdateWarehouseStatus(ctx, 999999, 0); !errors.Is(err, errcode.WarehouseNotFound) {
		t.Fatalf("update missing warehouse status: got %v, want %v", err, errcode.WarehouseNotFound)
	}
	if err := s.UpdateLocationStatus(ctx, 999999, basicmodel.LocationStatusIdle); !errors.Is(err, errcode.LocationNotFound) {
		t.Fatalf("update missing location: got %v, want %v", err, errcode.LocationNotFound)
	}
	if err := s.UpdateSKU(ctx, 999999, &dto.SKUReq{Code: "C", Barcode: "B", Name: "n"}); !errors.Is(err, errcode.SKUNotFound) {
		t.Fatalf("update missing sku: got %v, want %v", err, errcode.SKUNotFound)
	}
}

// 仓储层的按主键修改/删除统一通过 RequireAffected：0 行影响必须是 gorm.ErrRecordNotFound。
func TestRepositoryRequireAffectedOnMissingRows(t *testing.T) {
	s, _, _, db := newDeleteTestService(t)
	ctx := context.Background()

	if err := s.repo.UpdateWarehouse(ctx, db, 999999, "n", ""); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("repo.UpdateWarehouse: got %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if err := s.repo.DeleteWarehouse(ctx, db, 999999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("repo.DeleteWarehouse: got %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if err := s.repo.UpdateSKU(ctx, db, &basicmodel.SKU{Base: sysmodel.Base{ID: 999999}}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("repo.UpdateSKU: got %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if err := s.repo.DeleteLocation(ctx, db, 999999); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("repo.DeleteLocation: got %v, want %v", err, gorm.ErrRecordNotFound)
	}
	if err := s.repo.UpdateLocationStatusInTx(db, 999999, basicmodel.LocationStatusIdle); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("repo.UpdateLocationStatusInTx: got %v, want %v", err, gorm.ErrRecordNotFound)
	}
}
