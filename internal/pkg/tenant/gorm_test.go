package tenant

import (
	"context"
	"errors"
	"os"
	"testing"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/testutil"
)

// 集成测试：需要本地 MySQL；连不上自动跳过（与项目其他集成测试一致）。
func newTenantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &basicmodel.SKU{})
	if err := RegisterGORMCallbacks(db); err != nil {
		t.Fatalf("register tenant callbacks: %v", err)
	}
	return db
}

// TestTenantIsolation 多租户隔离核心回归：
// 两个租户各自插入数据后互相不可见；ctx 无租户（0）为平台旁路可见全部；
// 同一编码在不同租户可重复创建（联合唯一键）。
func TestTenantIsolation(t *testing.T) {
	db := newTenantTestDB(t)
	ctxA := WithTenant(context.Background(), 1001)
	ctxB := WithTenant(context.Background(), 2002)

	// Create：ctx 有租户且字段为零值时自动填充
	if err := db.WithContext(ctxA).Create(&basicmodel.SKU{Code: "SKU-T1", Barcode: "BC-T1", Name: "租户A货品"}).Error; err != nil {
		t.Fatalf("create tenant A sku: %v", err)
	}
	if err := db.WithContext(ctxB).Create(&basicmodel.SKU{Code: "SKU-T1", Barcode: "BC-T1", Name: "租户B货品"}).Error; err != nil {
		t.Fatalf("same code in tenant B should be allowed (composite unique): %v", err)
	}

	// Query：各租户只看到自己的数据
	var listA, listB []basicmodel.SKU
	if err := db.WithContext(ctxA).Find(&listA).Error; err != nil {
		t.Fatalf("query tenant A: %v", err)
	}
	if err := db.WithContext(ctxB).Find(&listB).Error; err != nil {
		t.Fatalf("query tenant B: %v", err)
	}
	if len(listA) != 1 || listA[0].TenantID != 1001 || listA[0].Name != "租户A货品" {
		t.Fatalf("tenant A should see exactly its own row, got %+v", listA)
	}
	if len(listB) != 1 || listB[0].TenantID != 2002 || listB[0].Name != "租户B货品" {
		t.Fatalf("tenant B should see exactly its own row, got %+v", listB)
	}

	// 平台旁路：无租户 ctx 可见全部
	var all []basicmodel.SKU
	if err := db.WithContext(context.Background()).Find(&all).Error; err != nil {
		t.Fatalf("query platform bypass: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("platform bypass should see all rows, got %d", len(all))
	}

	// Update/Delete：租户 A 的更新/删除不能影响租户 B 的行
	res := db.WithContext(ctxA).Model(&basicmodel.SKU{}).Where("code = ?", "SKU-T1").Update("name", "租户A改名")
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("tenant A update should affect only its own row, affected=%d err=%v", res.RowsAffected, res.Error)
	}
	var bRow basicmodel.SKU
	if err := db.WithContext(ctxB).Where("code = ?", "SKU-T1").First(&bRow).Error; err != nil {
		t.Fatalf("query tenant B after A update: %v", err)
	}
	if bRow.Name != "租户B货品" {
		t.Fatalf("tenant B row should be untouched, got name=%s", bRow.Name)
	}
	res = db.WithContext(ctxA).Where("code = ?", "SKU-T1").Delete(&basicmodel.SKU{})
	if res.Error != nil || res.RowsAffected != 1 {
		t.Fatalf("tenant A delete should affect only its own row, affected=%d err=%v", res.RowsAffected, res.Error)
	}
	var nB int64
	if err := db.WithContext(ctxB).Model(&basicmodel.SKU{}).Count(&nB).Error; err != nil {
		t.Fatalf("count tenant B after A delete: %v", err)
	}
	if nB != 1 {
		t.Fatalf("tenant B row must survive tenant A delete, got %d", nB)
	}

	// 租户请求不能通过显式字段跨租户创建；平台上下文仍支持代操作。
	explicit := basicmodel.SKU{TenantID: 3003, Code: "SKU-T2", Barcode: "BC-T2", Name: "显式租户"}
	if err := db.WithContext(ctxA).Create(&explicit).Error; !errors.Is(err, ErrMismatch) {
		t.Fatalf("cross-tenant create: %v", err)
	}
	if err := db.WithContext(context.Background()).Create(&explicit).Error; err != nil {
		t.Fatalf("create with explicit tenant: %v", err)
	}
	if explicit.TenantID != 3003 {
		t.Fatalf("explicit tenant id must not be overwritten, got %d", explicit.TenantID)
	}
}

// TestTenantScanIsolation 回归：Scan()/Rows() 走 Row 回调链而非 Query 链，
// Model+Scan(非 model 结构体) 的聚合/投影查询必须同样被租户过滤
// （AI 模块快照查询曾因此泄漏跨租户数据）。
func TestTenantScanIsolation(t *testing.T) {
	db := newTenantTestDB(t)
	ctxA := WithTenant(context.Background(), 1001)
	ctxB := WithTenant(context.Background(), 2002)

	for _, ctx := range []context.Context{ctxA, ctxB} {
		if err := db.WithContext(ctx).Create(&basicmodel.SKU{Code: "SCAN-T", Barcode: "SCAN-BC", Name: "扫描隔离"}).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	type brief struct {
		Code string
		Name string
	}
	var listA []brief
	if err := db.WithContext(ctxA).Model(&basicmodel.SKU{}).
		Select("code, name").Order("code").Scan(&listA).Error; err != nil {
		t.Fatalf("scan tenant A: %v", err)
	}
	if len(listA) != 1 || listA[0].Code != "SCAN-T" {
		t.Fatalf("tenant A scan should see only its own row, got %+v", listA)
	}
	var countB int64
	if err := db.WithContext(ctxB).Model(&basicmodel.SKU{}).Count(&countB).Error; err != nil {
		t.Fatalf("count tenant B: %v", err)
	}
	if countB != 1 {
		t.Fatalf("tenant B should see exactly its own row, got %d", countB)
	}
}

// TestWithExactTenantZeroDoesNotBypass 回归：平台租户 0 必须是精确范围，
// 不能和“未设置租户”的平台旁路混为一谈。
func TestWithExactTenantZeroDoesNotBypass(t *testing.T) {
	db := newTenantTestDB(t)

	if err := db.WithContext(context.Background()).Create(&basicmodel.SKU{
		TenantID: 0, Code: "PLATFORM", Barcode: "PLATFORM-BC", Name: "平台货品",
	}).Error; err != nil {
		t.Fatalf("create platform sku: %v", err)
	}
	if err := db.WithContext(WithTenant(context.Background(), 1001)).Create(&basicmodel.SKU{
		Code: "TENANT-1", Barcode: "TENANT-1-BC", Name: "租户货品",
	}).Error; err != nil {
		t.Fatalf("create tenant sku: %v", err)
	}

	var platformRows []basicmodel.SKU
	if err := db.WithContext(WithExactTenant(context.Background(), 0)).Find(&platformRows).Error; err != nil {
		t.Fatalf("query exact tenant 0: %v", err)
	}
	if len(platformRows) != 1 || platformRows[0].TenantID != 0 || platformRows[0].Code != "PLATFORM" {
		t.Fatalf("exact tenant 0 should see only platform rows, got %+v", platformRows)
	}

	var allRows []basicmodel.SKU
	if err := db.WithContext(WithTenant(context.Background(), 0)).Find(&allRows).Error; err != nil {
		t.Fatalf("query platform bypass: %v", err)
	}
	if len(allRows) != 2 {
		t.Fatalf("platform bypass should see all rows, got %d", len(allRows))
	}
}

func TestScope(t *testing.T) {
	if tenantID, scoped := Scope(context.Background()); scoped || tenantID != 0 {
		t.Fatalf("plain context scope=(%d,%v), want (0,false)", tenantID, scoped)
	}
	if tenantID, scoped := Scope(WithTenant(context.Background(), 0)); scoped || tenantID != 0 {
		t.Fatalf("tenant reset scope=(%d,%v), want (0,false)", tenantID, scoped)
	}
	if tenantID, scoped := Scope(WithExactTenant(context.Background(), 0)); !scoped || tenantID != 0 {
		t.Fatalf("exact tenant 0 scope=(%d,%v), want (0,true)", tenantID, scoped)
	}
	if tenantID, scoped := Scope(WithTenant(context.Background(), 42)); !scoped || tenantID != 42 {
		t.Fatalf("tenant 42 scope=(%d,%v), want (42,true)", tenantID, scoped)
	}
}

// TestTenantCreatePointerSliceAndBatches 回归：指针切片（[]*T）与 CreateInBatches
// 是 GORM 常见批量写法（入库单明细等），租户填充回调必须覆盖
// （曾因切片元素为指针未解引用导致明细 tenant_id=0，跨租户丢失单据）。
func TestTenantCreatePointerSliceAndBatches(t *testing.T) {
	db := newTenantTestDB(t)
	ctxA := WithTenant(context.Background(), 1001)

	// 指针切片普通 Create
	slice := []*basicmodel.SKU{
		{Code: "PTR-1", Barcode: "PTR-1", Name: "指针切片"},
		{Code: "PTR-2", Barcode: "PTR-2", Name: "指针切片"},
	}
	if err := db.WithContext(ctxA).Create(&slice).Error; err != nil {
		t.Fatalf("create pointer slice: %v", err)
	}
	for i, row := range slice {
		if row.TenantID != 1001 {
			t.Fatalf("pointer slice row %d tenant=%d, want 1001", i, row.TenantID)
		}
	}

	// CreateInBatches（单批与强制分批两种路径）
	batches := []*basicmodel.SKU{
		{Code: "BATCH-1", Barcode: "BATCH-1", Name: "批量"},
		{Code: "BATCH-2", Barcode: "BATCH-2", Name: "批量"},
	}
	if err := db.WithContext(ctxA).CreateInBatches(&batches, 100).Error; err != nil {
		t.Fatalf("create in batches: %v", err)
	}
	split := []*basicmodel.SKU{
		{Code: "SPLIT-1", Barcode: "SPLIT-1", Name: "分批"},
		{Code: "SPLIT-2", Barcode: "SPLIT-2", Name: "分批"},
	}
	if err := db.WithContext(ctxA).CreateInBatches(&split, 1).Error; err != nil {
		t.Fatalf("create in split batches: %v", err)
	}
	for _, group := range [][]*basicmodel.SKU{batches, split} {
		for _, row := range group {
			if row.TenantID != 1001 {
				t.Fatalf("batch row %s tenant=%d, want 1001", row.Code, row.TenantID)
			}
		}
	}

	// 值切片也一并覆盖
	values := []basicmodel.SKU{{Code: "VAL-1", Barcode: "VAL-1", Name: "值切片"}}
	if err := db.WithContext(ctxA).Create(&values).Error; err != nil {
		t.Fatalf("create value slice: %v", err)
	}
	if values[0].TenantID != 1001 {
		t.Fatalf("value slice row tenant=%d, want 1001", values[0].TenantID)
	}
}

// TestFromContext 覆盖与缺省语义。
func TestFromContext(t *testing.T) {
	if FromContext(context.TODO()) != 0 {
		t.Fatal("empty ctx should be 0")
	}
	if FromContext(context.Background()) != 0 {
		t.Fatal("plain ctx should be 0")
	}
	ctx := WithTenant(context.Background(), 42)
	if FromContext(ctx) != 42 {
		t.Fatalf("want 42, got %d", FromContext(ctx))
	}
	// 0 视为清除租户（旁路）
	ctx = WithTenant(ctx, 0)
	if FromContext(ctx) != 0 {
		t.Fatalf("want 0 after reset, got %d", FromContext(ctx))
	}
}
