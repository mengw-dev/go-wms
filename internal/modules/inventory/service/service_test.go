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

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

// 集成测试通过 WMS_TEST_DSN 连接 MySQL，并在独立临时库中运行。
// WMS_TEST_REQUIRED=1 时连接失败会让测试失败，否则跳过。
func newTestService(t *testing.T) (*Service, *tx.Manager, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?charset=utf8mb4&parseTime=True&loc=Local"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn,
		&model.Inventory{}, &model.InventoryTrans{}, &basicmodel.Warehouse{}, &basicmodel.Location{}, &basicmodel.SKU{})
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}
	tm := tx.New(db)
	return New(repository.New(), tm), tm, db
}

func setupStock(t *testing.T, svc *Service, tm *tx.Manager, locationID, skuID int64, batchNo string, qty int) int64 {
	t.Helper()
	ctx := context.Background()
	whID := snowflake.Next() // 随机仓库 ID 隔离测试数据
	err := tm.Tx(ctx, func(tx *gorm.DB) error {
		if err := tx.Create(&basicmodel.Warehouse{
			Base: sysmodel.Base{ID: whID}, Code: fmt.Sprintf("W-%d", whID), Name: "test", Status: 1,
		}).Error; err != nil {
			return err
		}
		if err := tx.Create(&basicmodel.SKU{
			Base: sysmodel.Base{ID: skuID}, Code: fmt.Sprintf("S-%d", skuID), Barcode: fmt.Sprintf("B-%d", skuID),
			Name: "test", Unit: "件", Status: 1,
		}).Error; err != nil {
			return err
		}
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
// 失败可能是"库存不足"（快照能确证不足），也可能是"数据并发冲突"——
// 锁到的行比候选快照少时按快照新鲜度优先，交外层换新事务重试（生产入口 Approve 有 TxRetry）。
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
			switch {
			case err == nil:
				success.Add(1)
			case errcode.From(err).Code == errcode.AvailableNotEnough.Code:
				fail.Add(1)
			case errcode.From(err).Code == errcode.Conflict.Code:
				// 快照已过期（别人抢走了库存）→ 可重试的业务拒绝；
				// 生产入口用 TxRetry 换新快照重试，重试后仍不足才会报库存不足。
				// 这里直接调用 tm.Tx，40900 出现属预期，超卖仍由下方不变量断言兜底。
				fail.Add(1)
			default:
				t.Errorf("allocate %d: unexpected error: %v", i, err)
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

// TestAllocateOnlyLocksNeededRows 验证锁范围收敛：
// 分配只需要一个批次时，其它未被使用的库存行不应被本次事务锁住。
// 旧实现会锁住该 SKU 的全部可用行，本用例在那时会因第二个事务拿不到锁而失败。
func TestAllocateOnlyLocksNeededRows(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := setupStock(t, svc, tm, locID, skuID, "B01", 10)

	// 第二批：直接落库，避免 Increase 的 1 秒等待；入库时间晚于第一批。
	secondID := snowflake.Next()
	if err := db.Create(&model.Inventory{
		Base: sysmodel.Base{ID: secondID}, WarehouseID: whID, LocationID: locID, SKUID: skuID,
		BatchNo: "B02", StockQuantity: 10, AvailableQty: 10,
		StockInTime: time.Now().Add(time.Minute),
	}).Error; err != nil {
		t.Fatalf("seed second batch: %v", err)
	}

	started := make(chan struct{})
	proceed := make(chan struct{})
	allocDone := make(chan error, 1)
	go func() {
		allocDone <- tm.Tx(ctx, func(tx *gorm.DB) error {
			if _, err := svc.Allocate(ctx, tx, &api.AllocateReq{
				WarehouseID: whID, SKUID: skuID, Quantity: 5, OrderNo: "CK-LOCK", Operator: "test",
			}); err != nil {
				return err
			}
			close(started)
			<-proceed // 保持事务打开，让另一个连接尝试加锁
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(proceed)
		t.Fatal("allocate did not reach the holding point")
	}

	// 另一个连接尝试锁第二批：第一批就够本次分配，这里不应被阻塞。
	lockCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	var other model.Inventory
	lockErr := db.WithContext(lockCtx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", secondID).First(&other).Error
	cancel()
	close(proceed)
	if err := <-allocDone; err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if lockErr != nil {
		t.Fatalf("本次分配未使用第二批，却把它的行锁住了（锁范围未收敛）: %v", lockErr)
	}
}

// TestAllocateAcrossManyBatches 验证跨批次分配：
// 60 个批次各 1 件，申请 5 件，应当按 FIFO 落在最早的 5 个批次上，其余批次保持不动。
func TestAllocateAcrossManyBatches(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := setupStock(t, svc, tm, locID, skuID, "B000", 1)

	base := time.Now().Add(time.Minute) // 晚于 setupStock 写入的第一批
	rest := make([]*model.Inventory, 0, 59)
	for i := 1; i < 60; i++ {
		rest = append(rest, &model.Inventory{
			Base: sysmodel.Base{ID: snowflake.Next()}, WarehouseID: whID, LocationID: locID, SKUID: skuID,
			BatchNo: fmt.Sprintf("B%03d", i), StockQuantity: 1, AvailableQty: 1,
			StockInTime: base.Add(time.Duration(i) * time.Second),
		})
	}
	if err := db.Create(&rest).Error; err != nil {
		t.Fatalf("seed batches: %v", err)
	}

	var result *api.AllocateResult
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		r, err := svc.Allocate(ctx, tx, &api.AllocateReq{
			WarehouseID: whID, SKUID: skuID, Quantity: 5, OrderNo: "CK-MANY", Operator: "test",
		})
		result = r
		return err
	}); err != nil {
		t.Fatalf("allocate: %v", err)
	}
	if len(result.Rows) != 5 {
		t.Fatalf("expect 5 allocate rows, got %d", len(result.Rows))
	}
	for i, row := range result.Rows {
		want := fmt.Sprintf("B%03d", i)
		if row.BatchNo != want || row.Quantity != 1 {
			t.Fatalf("row %d expect %s/1, got %s/%d", i, want, row.BatchNo, row.Quantity)
		}
	}

	var sum struct {
		Available int `gorm:"column:available"`
	}
	if err := db.Model(&model.Inventory{}).Where("warehouse_id = ? AND sku_id = ?", whID, skuID).
		Select("COALESCE(SUM(available_quantity), 0) AS available").Scan(&sum).Error; err != nil {
		t.Fatalf("sum available: %v", err)
	}
	if sum.Available != 55 {
		t.Fatalf("expect 55 remaining available, got %d", sum.Available)
	}
}

// TestAllocatePagesBeyondBatchWindow 回归：候选行数远超单批窗口（20 行/批）时，
// 只要库存充足就必须继续翻页完成分配，不能因为"批数"上限被误判成并发冲突。
// 场景：130 个批次各 1 件，申请 110 件（旧实现最多处理 5 批 = 100 行 → 必然失败）。
func TestAllocatePagesBeyondBatchWindow(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: 1, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	whID := setupStock(t, svc, tm, locID, skuID, "B000", 1)

	base := time.Now().Add(time.Minute) // 晚于 setupStock 写入的第一批
	rest := make([]*model.Inventory, 0, 129)
	for i := 1; i < 130; i++ {
		rest = append(rest, &model.Inventory{
			Base: sysmodel.Base{ID: snowflake.Next()}, WarehouseID: whID, LocationID: locID, SKUID: skuID,
			BatchNo: fmt.Sprintf("B%03d", i), StockQuantity: 1, AvailableQty: 1,
			StockInTime: base.Add(time.Duration(i) * time.Second),
		})
	}
	if err := db.Create(&rest).Error; err != nil {
		t.Fatalf("seed batches: %v", err)
	}

	var result *api.AllocateResult
	if err := tm.Tx(ctx, func(tx *gorm.DB) error {
		r, err := svc.Allocate(ctx, tx, &api.AllocateReq{
			WarehouseID: whID, SKUID: skuID, Quantity: 110, OrderNo: "CK-PAGE", Operator: "test",
		})
		result = r
		return err
	}); err != nil {
		t.Fatalf("库存充足时不应因批次窗口误判失败: %v", err)
	}
	if len(result.Rows) != 110 || result.Total != 110 {
		t.Fatalf("expect 110 rows/110 total, got %d/%d", len(result.Rows), result.Total)
	}
	// FIFO：最早的 110 个批次，按入库时间升序
	for i, row := range result.Rows {
		want := fmt.Sprintf("B%03d", i)
		if row.BatchNo != want || row.Quantity != 1 {
			t.Fatalf("row %d expect %s/1, got %s/%d", i, want, row.BatchNo, row.Quantity)
		}
	}

	var sum struct {
		Available int `gorm:"column:available"`
	}
	if err := db.Model(&model.Inventory{}).Where("warehouse_id = ? AND sku_id = ?", whID, skuID).
		Select("COALESCE(SUM(available_quantity), 0) AS available").Scan(&sum).Error; err != nil {
		t.Fatalf("sum available: %v", err)
	}
	if sum.Available != 20 {
		t.Fatalf("expect 20 remaining available, got %d", sum.Available)
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
		t.Fatalf("final expect 30/30/0, got %d/%d/%d", inv.StockQuantity, inv.AvailableQty, inv.AllocatedQty)
	}

	// 超发防护：再发货 15（已分配仅 0）必须失败
	err := tm.Tx(ctx, func(tx *gorm.DB) error {
		return svc.Ship(ctx, tx, &api.ShipReq{InventoryID: invID, Quantity: 15, OrderNo: "CK-2", Operator: "test"})
	})
	if err == nil {
		t.Fatal("oversell ship should fail")
	}
	check("oversell-ship")
	// 每次成功变更都有流水，包括只改变可用/分配量的审核操作；失败发货没有流水。
	var flows []*model.InventoryTrans
	if err := db.Where("inventory_id = ?", invID).Order("id").Find(&flows).Error; err != nil {
		t.Fatal(err)
	}
	if len(flows) != 4 {
		t.Fatalf("flows=%d want=4", len(flows))
	}
	for i, typ := range []model.TransType{model.TransReceive, model.TransAllocate, model.TransRelease, model.TransShip} {
		if flows[i].TransType != typ {
			t.Fatalf("flow %d type=%s want=%s", i, flows[i].TransType, typ)
		}
		if i > 0 && (flows[i].BeforeQuantity != flows[i-1].AfterQuantity || flows[i].AvailableBefore != flows[i-1].AvailableAfter) {
			t.Fatalf("flow chain is broken at %d", i)
		}
	}
}

// TestConcurrentIncreaseTransFlow 并发上架流水一致性：
// 8 个并发各向相同"仓库+库位+SKU+批次"上架 10（共 80），
// 要求最终库存准确，且 RECEIVE 流水的 Before/After 首尾相接无缺口、无重叠（可对账）。
func TestConcurrentIncreaseTransFlow(t *testing.T) {
	svc, tm, db := newTestService(t)
	ctx := context.Background()

	whID := snowflake.Next()
	locID := snowflake.Next()
	if err := db.Create(&basicmodel.Location{
		Base: sysmodel.Base{ID: locID}, WarehouseID: whID, Code: fmt.Sprintf("T-%d", locID), Status: 1,
	}).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	skuID := snowflake.Next()
	if err := db.Create(&basicmodel.Warehouse{
		Base: sysmodel.Base{ID: whID}, Code: fmt.Sprintf("W-%d", whID), Name: "test", Status: 1,
	}).Error; err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	if err := db.Create(&basicmodel.SKU{
		Base: sysmodel.Base{ID: skuID}, Code: fmt.Sprintf("S-%d", skuID), Barcode: fmt.Sprintf("B-%d", skuID),
		Name: "test", Unit: "件", Status: 1,
	}).Error; err != nil {
		t.Fatalf("create sku: %v", err)
	}

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
