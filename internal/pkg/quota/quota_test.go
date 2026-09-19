package quota

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

// 集成测试：需要本地 MySQL；连不上自动跳过（与项目其他集成测试一致）。
func newQuotaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &basicmodel.SKU{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatalf("register tenant callbacks: %v", err)
	}
	return db
}

func seedSKU(t *testing.T, db *gorm.DB, ctx context.Context, code string) {
	t.Helper()
	// 条码在租户内唯一（uk_sku_barcode），测试数据按编码派生避免撞唯一键
	if err := db.WithContext(ctx).Create(&basicmodel.SKU{Code: code, Barcode: "BC-" + code, Name: "货品 " + code}).Error; err != nil {
		t.Fatalf("seed sku %s: %v", code, err)
	}
}

// TestGuardBlocksWhenLimitReached 达到上限后拒绝继续写入，错误码为 90001；放宽上限后恢复。
func TestGuardBlocksWhenLimitReached(t *testing.T) {
	db := newQuotaTestDB(t)
	ctx := tenant.WithTenant(context.Background(), 1001)
	seedSKU(t, db, ctx, "SKU-1")
	seedSKU(t, db, ctx, "SKU-2")

	err := Guard(ctx, db, &basicmodel.SKU{}, 2, 1, "货品")
	var bizErr *errcode.Error
	if !errors.As(err, &bizErr) || bizErr.Code != errcode.QuotaExceeded.Code {
		t.Fatalf("expect quota exceeded (90001), got %v", err)
	}
	if err := Guard(ctx, db, &basicmodel.SKU{}, 3, 1, "货品"); err != nil {
		t.Fatalf("under limit should pass, got %v", err)
	}
}

// TestGuardCountsWithinTenantOnly 计数只统计当前租户：其他租户的数据不占用本租户额度。
func TestGuardCountsWithinTenantOnly(t *testing.T) {
	db := newQuotaTestDB(t)
	ctxA := tenant.WithTenant(context.Background(), 1001)
	ctxB := tenant.WithTenant(context.Background(), 2002)
	seedSKU(t, db, ctxA, "SKU-A")

	if err := Guard(ctxB, db, &basicmodel.SKU{}, 1, 1, "货品"); err != nil {
		t.Fatalf("tenant B has its own quota, got %v", err)
	}
}

// TestGuardSkipsPlatformAndUnlimited 平台租户（tenant_id<=0）与 limit<=0 均不限制。
func TestGuardSkipsPlatformAndUnlimited(t *testing.T) {
	db := newQuotaTestDB(t)
	if err := Guard(context.Background(), db, &basicmodel.SKU{}, 1, 5, "货品"); err != nil {
		t.Fatalf("platform tenant should bypass quota, got %v", err)
	}
	ctx := tenant.WithTenant(context.Background(), 1001)
	seedSKU(t, db, ctx, "SKU-X")
	if err := Guard(ctx, db, &basicmodel.SKU{}, 0, 10, "货品"); err != nil {
		t.Fatalf("limit<=0 means unlimited, got %v", err)
	}
}

// TestGuardImportRows 单次导入行数上限：超过上限拒绝，平台租户不受限。
func TestGuardImportRows(t *testing.T) {
	ctx := tenant.WithTenant(context.Background(), 1001)
	err := GuardImportRows(ctx, 5, 3)
	var bizErr *errcode.Error
	if !errors.As(err, &bizErr) || bizErr.Code != errcode.QuotaExceeded.Code {
		t.Fatalf("expect quota exceeded for oversized import, got %v", err)
	}
	if err := GuardImportRows(ctx, 3, 3); err != nil {
		t.Fatalf("rows equal to limit should pass, got %v", err)
	}
	if err := GuardImportRows(context.Background(), 100, 3); err != nil {
		t.Fatalf("platform tenant should bypass import limit, got %v", err)
	}
	if err := GuardImportRows(ctx, 5, 3); !strings.Contains(err.Error(), "5") {
		t.Fatalf("error message should mention actual rows, got %v", err)
	}
}
