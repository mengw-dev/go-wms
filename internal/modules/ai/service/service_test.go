package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/testutil"
)

// 集成测试：需要本地 MySQL（默认 root:1234@127.0.0.1:3306）。
// 可通过环境变量 WMS_TEST_DSN 覆盖；连不上数据库时自动跳过（与项目其他集成测试一致）。
func newTestService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn,
		&basicmodel.Warehouse{}, &basicmodel.Location{}, &basicmodel.SKU{}, &invmodel.Inventory{})
	s := New(config.AIConfig{RateLimitPerMin: 10}, db, nil)
	return s, db
}

// seedSnapshotData 写入最小可断言数据集：1 仓库 + 2 库位 + 2 SKU + 2 条库存。
func seedSnapshotData(t *testing.T, db *gorm.DB) (whID, locA, locB, skuA, skuB int64) {
	t.Helper()
	wh := basicmodel.Warehouse{Code: "WH01", Name: "测试一号仓", Status: 1}
	loc1 := basicmodel.Location{WarehouseID: 0, Code: "A01-01-01", Zone: "A01", Status: 1}
	loc2 := basicmodel.Location{WarehouseID: 0, Code: "A01-01-02", Zone: "A01", Status: 1}
	sku1 := basicmodel.SKU{Code: "SKU900001", Barcode: "6909000000011", Name: "测试矿泉水", Spec: "500ml", Unit: "箱", Status: 1}
	sku2 := basicmodel.SKU{Code: "SKU900002", Barcode: "6909000000028", Name: "测试方便面", Spec: "100g", Unit: "箱", Status: 1}
	if err := db.Create(&wh).Error; err != nil {
		t.Fatalf("seed warehouse: %v", err)
	}
	loc1.WarehouseID, loc2.WarehouseID = wh.ID, wh.ID
	if err := db.Create(&loc1).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	if err := db.Create(&loc2).Error; err != nil {
		t.Fatalf("seed location: %v", err)
	}
	if err := db.Create(&sku1).Error; err != nil {
		t.Fatalf("seed sku: %v", err)
	}
	if err := db.Create(&sku2).Error; err != nil {
		t.Fatalf("seed sku: %v", err)
	}
	invs := []invmodel.Inventory{
		{WarehouseID: wh.ID, LocationID: loc1.ID, SKUID: sku1.ID, BatchNo: "B001",
			StockQuantity: 100, AvailableQty: 100, StockInTime: time.Now()},
		{WarehouseID: wh.ID, LocationID: loc2.ID, SKUID: sku2.ID, BatchNo: "B002",
			StockQuantity: 10, AvailableQty: 5, AllocatedQty: 5, StockInTime: time.Now()},
	}
	if err := db.Create(&invs).Error; err != nil {
		t.Fatalf("seed inventory: %v", err)
	}
	return wh.ID, loc1.ID, loc2.ID, sku1.ID, sku2.ID
}

// TestBuildSnapshot 验证检索增强的快照包含真实数据（概览/仓库分布/Top/低可用/目录/明细）。
func TestBuildSnapshot(t *testing.T) {
	s, db := newTestService(t)
	_, _, _, _, _ = seedSnapshotData(t, db)

	snapshot, err := s.buildSnapshot(context.Background(), "SKU900001 的库存明细")
	if err != nil {
		t.Fatalf("buildSnapshot: %v", err)
	}

	asserts := []string{
		"SKU 总数: 2",               // 概览：SKU 计数不被聚合 Scan 清零（回归点）
		"库存总量: 110",               // 100 + 10
		"测试一号仓(WH01)",             // 仓库分布
		"SKU900001 测试矿泉水: 库存 100", // Top SKU
		"SKU900002 测试方便面: 可用 5",   // 低可用（阈值 50 以内）
		"SKU900001 | 测试矿泉水",       // SKU 目录
		"库位 A01-01-01，批次 B001",    // 问题匹配到的 SKU 明细
	}
	for _, want := range asserts {
		if !strings.Contains(snapshot, want) {
			t.Errorf("snapshot 缺少期望内容 %q\n--- snapshot ---\n%s", want, snapshot)
		}
	}
}

// TestChatKeyMissing 未配置 API Key 时返回明确业务错误（不触发任何外部调用）。
func TestChatKeyMissing(t *testing.T) {
	s, _ := newTestService(t) // AIConfig.APIKey 为空
	_, err := s.Chat(context.Background(), 1, "库存总量是多少")
	if !errors.Is(err, errcode.AIKeyMissing) {
		t.Fatalf("expect AIKeyMissing, got %v", err)
	}
}

// TestAllowRateRedisNil Redis 客户端为 nil（未初始化/降级）时限流自动放行。
func TestAllowRateRedisNil(t *testing.T) {
	s, _ := newTestService(t)
	if !s.allowRate(context.Background(), 1) {
		t.Fatal("nil redis should allow (degrade)")
	}
}
