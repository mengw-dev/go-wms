package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
)

// 集成测试：需要本地 MySQL（默认 root:root123@127.0.0.1:3306/gowms）。
// 可通过环境变量 WMS_TEST_DSN 覆盖；连不上数据库时自动跳过。
func newTestService(t *testing.T) (*Service, *tx.Manager, *gorm.DB) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
	})
	if err != nil {
		t.Skipf("mysql unavailable, skip: %v", err)
	}
	if err := db.AutoMigrate(&model.Inventory{}, &model.InventoryTrans{}, &basicmodel.Location{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	snowflake.Init(1)
	tm := tx.New(db)
	return New(repository.New(), tm), tm, db
}

func setupStock(t *testing.T, svc *Service, tm *tx.Manager, locationID, skuID int64, batchNo string, qty int) int64 {
	t.Helper()
	ctx := context.Background()
	whID := snowflake.Next() // 随机仓库 ID 隔离测试数据
	err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Increase(ctx, tx, &api.IncreaseReq{
			WarehouseID: whID, LocationID: locationID, SKUID: skuID,
			BatchNo: batchNo, Quantity: qty, OrderNo: "TEST", Operator: "test",
		})
	})
	if err != nil {
		t.Fatalf("seed stock: %v", err)
	}
	return whID
}

func getInv(t *testing.T, db *gorm.DB, whID, skuID int64) *model.Inventory {
	t.Helper()
	var inv model.Inventory
	if err := db.Where("warehouse_id = ? AND sku_id = ?", whID, skuID).First(&inv).Error; err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	return &inv
}

// TestConcurrentAllocateAntiOversell 并发分配防超卖：
// 100 个可用库存，200 个并发各分配 1，要求恰好成功 100 次、失败 100 次，
// 且最终满足 stock = available + allocated，任何数量不允许为负。
func TestConcurrentAllocateAntiOversell(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := setupStock(t, svc, tm, locID, skuID, "B202601", 100)

	const goroutines = 200
	var success, fail atomic.Int64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			err := tm.Tx(ctx, func(tx *gorm.DB) error {
				_, err := svc.Allocate(ctx, tx, &api.AllocateReq{
					WarehouseID: whID, SKUID: skuID, Quantity: 1,
					OrderNo: fmt.Sprintf("CK-TEST-%d", i), Operator: "test",
				})
				return err
			})
			if err == nil {
				success.Add(1)
			} else {
				fail.Add(1)
			}
		}(i)
	}
	close(start)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent allocate timeout")
	}

	if success.Load() != 100 || fail.Load() != 100 {
		t.Fatalf("expect success=100 fail=100, got success=%d fail=%d", success.Load(), fail.Load())
	}
	inv := getInv(t, db, whID, skuID)
	if inv.StockQuantity != 100 || inv.AvailableQty != 0 || inv.AllocatedQty != 100 {
		t.Fatalf("expect stock=100 available=0 allocated=100, got %d/%d/%d",
			inv.StockQuantity, inv.AvailableQty, inv.AllocatedQty)
	}
	if inv.StockQuantity != inv.AvailableQty+inv.AllocatedQty {
		t.Fatalf("invariant broken: %d != %d + %d", inv.StockQuantity, inv.AvailableQty, inv.AllocatedQty)
	}
}

// TestAllocateFIFO 验证 FIFO：先入库的批次先被分配，跨批次取数正确。
func TestAllocateFIFO(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := setupStock(t, svc, tm, locID, skuID, "FIRST", 30)
	// 第二批：晚 1 秒入库保证 FIFO 顺序
	time.Sleep(time.Second)
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Increase(ctx, tx, &api.IncreaseReq{
			WarehouseID: whID, LocationID: locID, SKUID: skuID,
			BatchNo: "SECOND", Quantity: 80, OrderNo: "TEST2", Operator: "test",
		})
	}); err != nil {
		t.Fatalf("seed second batch: %v", err)
	}

	var result *api.AllocateResult
	err := tm.Tx(ctx, func(tx *gorm.DB) error {
		r, err := svc.Allocate(ctx, tx, &api.AllocateReq{
			WarehouseID: whID, SKUID: skuID, Quantity: 100, OrderNo: "CK-FIFO", Operator: "test",
		})
		result = r
		return err
	})
	if err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expect 2 allocate rows, got %d", len(result.Rows))
	}
	first, second := result.Rows[0], result.Rows[1]
	if first.BatchNo != "FIRST" || first.Quantity != 30 {
		t.Fatalf("FIFO row1 expect FIRST/30, got %s/%d", first.BatchNo, first.Quantity)
	}
	if second.BatchNo != "SECOND" || second.Quantity != 70 {
		t.Fatalf("FIFO row2 expect SECOND/70, got %s/%d", second.BatchNo, second.Quantity)
	}
}

// TestShipReleaseInvariant 验证分配→释放→发货全程满足三数量恒等式且不为负。
func TestShipReleaseInvariant(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := setupStock(t, svc, tm, locID, skuID, "B1", 50)

	check := func(stage string) {
		t.Helper()
		inv := getInv(t, db, whID, skuID)
		if inv.StockQuantity < 0 || inv.AvailableQty < 0 || inv.AllocatedQty < 0 {
			t.Fatalf("[%s] negative quantity: %d/%d/%d", stage, inv.StockQuantity, inv.AvailableQty, inv.AllocatedQty)
		}
		if inv.StockQuantity != inv.AvailableQty+inv.AllocatedQty {
			t.Fatalf("[%s] invariant broken: %d != %d + %d", stage, inv.StockQuantity, inv.AvailableQty, inv.AllocatedQty)
		}
	}
	check("seed")

	// 分配 30
	var allocRes *api.AllocateResult
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		r, err := svc.Allocate(ctx, tx, &api.AllocateReq{WarehouseID: whID, SKUID: skuID, Quantity: 30, OrderNo: "CK-1", Operator: "test"})
		allocRes = r
		return err
	}); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	check("allocate")
	if allocRes.Total != 30 {
		t.Fatalf("allocate total expect 30, got %d", allocRes.Total)
	}

	// 释放 10
	invID := allocRes.Rows[0].InventoryID
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Release(ctx, tx, &api.ReleaseReq{InventoryID: invID, Quantity: 10, OrderNo: "CK-1", Operator: "test"})
	}); err != nil {
		t.Fatalf("release: %v", err)
	}
	check("release")

	// 发货 20
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Ship(ctx, tx, &api.ShipReq{InventoryID: invID, Quantity: 20, OrderNo: "CK-1", Operator: "test"})
	}); err != nil {
		t.Fatalf("ship: %v", err)
	}
	check("ship")

	// 期望：stock=30 available=30 allocated=0（50-发货20，分配30已释放10后全部发货）
	inv := getInv(t, db, whID, skuID)
	if inv.StockQuantity != 30 || inv.AvailableQty != 30 || inv.AllocatedQty != 0 {
		t.Fatalf("final expect 30/20/10, got %d/%d/%d", inv.StockQuantity, inv.AvailableQty, inv.AllocatedQty)
	}

	// 超发防护：再发货 15（已分配仅 0）必须失败
	err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Ship(ctx, tx, &api.ShipReq{InventoryID: invID, Quantity: 15, OrderNo: "CK-2", Operator: "test"})
	})
	if err == nil {
		t.Fatal("oversell ship should fail")
	}
	check("oversell-ship")
}

// TestConcurrentIncreaseTransFlow 并发上架流水一致性：
// 8 个并发各向相同"仓库+库位+SKU+批次"上架 10（共 80），
// 要求最终库存准确，且 RECEIVE 流水的 Before/After 首尾相接无缺口、无重叠（可对账）。
func TestConcurrentIncreaseTransFlow(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := snowflake.Next()

	const goroutines, perQty = 8, 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// TxRetry：首次并发创建同四元组可能因 gap lock 死锁，整事务重试后走锁读累加
			if err := tm.TxRetry(ctx, tx.MaxTxRetry, func(tx *gorm.DB) error {
				return svc.Increase(ctx, tx, &api.IncreaseReq{
					WarehouseID: whID, LocationID: locID, SKUID: skuID,
					BatchNo: "B-CONC", Quantity: perQty,
					OrderNo: fmt.Sprintf("RK-TEST-%d", i), Operator: "test",
				})
			}); err != nil {
				t.Errorf("increase %d: %v", i, err)
			}
		}(i)
	}
	close(start)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("concurrent increase timeout")
	}

	// 最终库存：stock = available = 80
	var inv model.Inventory
	if err := db.Where("warehouse_id = ? AND sku_id = ?", whID, skuID).First(&inv).Error; err != nil {
		t.Fatalf("load inventory: %v", err)
	}
	total := goroutines * perQty
	if inv.StockQuantity != total || inv.AvailableQty != total {
		t.Fatalf("expect stock=%d available=%d, got %d/%d", total, total, inv.StockQuantity, inv.AvailableQty)
	}

	// 流水链校验：按 BeforeQuantity 排序后应首尾相接覆盖 [0, total]
	var trans []model.InventoryTrans
	if err := db.Where("inventory_id = ?", inv.ID).Where("trans_type = ?", model.TransReceive).Find(&trans).Error; err != nil {
		t.Fatalf("load trans: %v", err)
	}
	if len(trans) != goroutines {
		t.Fatalf("expect %d trans rows, got %d", goroutines, len(trans))
	}
	sort.Slice(trans, func(a, b int) bool { return trans[a].BeforeQuantity < trans[b].BeforeQuantity })
	expect := 0
	for _, tr := range trans {
		if tr.BeforeQuantity != expect || tr.AfterQuantity != expect+perQty {
			t.Fatalf("trans chain broken at before=%d: expect [%d,%d), got [%d,%d)",
				expect, expect, expect+perQty, tr.BeforeQuantity, tr.AfterQuantity)
		}
		if tr.AvailableBefore != expect || tr.AvailableAfter != expect+perQty {
			t.Fatalf("available chain broken: expect [%d,%d), got [%d,%d)",
				expect, expect+perQty, tr.AvailableBefore, tr.AvailableAfter)
		}
		expect = tr.AfterQuantity
	}
	if expect != total {
		t.Fatalf("trans chain total expect %d, got %d", total, expect)
	}
}
