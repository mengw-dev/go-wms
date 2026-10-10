package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

// 本文件内使用 stocktakeFixture / countedOrder（service_test.go）与真实 MySQL。

// TestStocktakeApproveIdempotencyReplay 审核成功后的同 key 重试回放空成功：
// 已终态（COMPLETED）不再被误报为 StocktakeStatusWrong，库存/流水/明细不重复调整。
// 这补的是请求重放语义——终态防重复调整仍由状态机与明细 adjusted 标记保证。
func TestStocktakeApproveIdempotencyReplay(t *testing.T) {
	s, db, ctx, inventory := stocktakeFixture(t)
	o, _ := countedOrder(ctx, t, s, 8)

	if err := s.Approve(ctx, o.ID, "test", "appr-key-1"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	// 库存从 10 调整到 8，产生一条 ADJUST 流水
	if err := db.WithContext(ctx).First(inventory, inventory.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.StockQuantity != 8 || inventory.AvailableQty != 8 {
		t.Fatalf("inventory after approve=%+v", inventory)
	}
	var transBefore int64
	if err := db.WithContext(ctx).Model(&invmodel.InventoryTrans{}).Count(&transBefore).Error; err != nil {
		t.Fatal(err)
	}
	// 终态后同 key 重试：回放空成功，不再是 60002；库存与流水完全不变
	for i := range 3 {
		if err := s.Approve(ctx, o.ID, "test", "appr-key-1"); err != nil {
			t.Fatalf("replay %d: %v", i, err)
		}
	}
	var transAfter int64
	if err := db.WithContext(ctx).Model(&invmodel.InventoryTrans{}).Count(&transAfter).Error; err != nil {
		t.Fatal(err)
	}
	if transAfter != transBefore {
		t.Fatalf("trans changed: before=%d after=%d", transBefore, transAfter)
	}
	if err := db.WithContext(ctx).First(inventory, inventory.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.StockQuantity != 8 {
		t.Fatalf("stock after replays=%d want=8", inventory.StockQuantity)
	}
	var detail model.StocktakeDetail
	if err := db.WithContext(ctx).Where("order_id = ?", o.ID).First(&detail).Error; err != nil {
		t.Fatal(err)
	}
	if !detail.Adjusted {
		t.Fatal("detail should remain adjusted")
	}
}

// TestStocktakeApproveBusinessFailureReusesKey 业务拒绝（未录全实盘）不留下幂等记录，
// 补录后同一 key 作为新操作执行成功。
func TestStocktakeApproveBusinessFailureReusesKey(t *testing.T) {
	s, _, ctx, _ := stocktakeFixture(t)
	o, err := s.Create(ctx, &dto.CreateOrderReq{WarehouseID: 1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Approve(ctx, o.ID, "test", "appr-key-fail"); !errors.Is(err, errcode.StocktakeNotFullyCounted) {
		t.Fatalf("approve before counting: %v", err)
	}
	detail, err := s.Get(ctx, o.ID)
	if err != nil || len(detail.Details) != 1 {
		t.Fatalf("details=%+v err=%v", detail, err)
	}
	if err := s.RecordActual(ctx, o.ID, detail.Details[0].ID, 8); err != nil {
		t.Fatal(err)
	}
	if err := s.Approve(ctx, o.ID, "test", "appr-key-fail"); err != nil {
		t.Fatalf("approve with same key after rollback: %v", err)
	}
}

// TestStocktakeApproveKeyReusedAcrossOrders 同一 key 用于另一张盘点的审核 → 409，
// 不能悄悄按新操作执行。
func TestStocktakeApproveKeyReusedAcrossOrders(t *testing.T) {
	s, _, ctx, _ := stocktakeFixture(t)
	first, _ := countedOrder(ctx, t, s, 8)
	if err := s.Approve(ctx, first.ID, "test", "appr-key-x"); err != nil {
		t.Fatalf("first approve: %v", err)
	}
	second, _ := countedOrder(ctx, t, s, 9)
	if err := s.Approve(ctx, second.ID, "test", "appr-key-x"); !errors.Is(err, errcode.IdempotencyKeyReused) {
		t.Fatalf("key reused across orders: %v", err)
	}
	if result, err := s.Get(ctx, second.ID); err != nil || result.Order.Status == model.OrderCompleted {
		t.Fatalf("second order must not complete: resp=%+v err=%v", result, err)
	}
}

// TestStocktakeApproveCorruptAndIsolation 损坏空成功标记返回内部错误；跨租户/跨 scope 同 key 隔离。
func TestStocktakeApproveCorruptAndIsolation(t *testing.T) {
	s, db, ctx, _ := stocktakeFixture(t)
	o, _ := countedOrder(ctx, t, s, 8)
	if err := s.Approve(ctx, o.ID, "test", "appr-key-corrupt"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	for _, shape := range []string{`null`, `{"x":1}`, ``} {
		if err := db.WithContext(ctx).Model(&idempotency.Record{}).
			Where("idempotency_key = ?", "appr-key-corrupt").Update("result_json", shape).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.Approve(ctx, o.ID, "test", "appr-key-corrupt"); !errors.Is(err, errcode.Internal) {
			t.Fatalf("shape %q: %v", shape, err)
		}
	}

	// 跨租户/跨 scope 的同 key fixture 不影响本租户新操作
	fixtures := []*idempotency.Record{
		{TenantID: 12, Scope: "stocktake.approve", IdempotencyKey: "appr-key-iso"},
		{TenantID: 11, Scope: "other.scope", IdempotencyKey: "appr-key-iso"},
	}
	for _, fixture := range fixtures {
		fixture.ID = snowflake.Next()
		fixture.RequestHash = "other-hash"
		fixture.ObjectID = o.ID
		fixture.ResultJSON = idempotency.EmptySuccessJSON
		// 平台旁路上下文写入跨租户 fixture：正租户上下文不允许显式写其他 tenant_id。
		if err := db.WithContext(tenant.WithTenant(context.Background(), 0)).Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}
	other, _ := countedOrder(ctx, t, s, 7)
	if err := s.Approve(ctx, other.ID, "test", "appr-key-iso"); err != nil {
		t.Fatalf("isolated approve: %v", err)
	}
}

// TestStocktakeApproveUsesLatestActualQty 审核与实盘录入交错时，调整必须基于最新实盘数量：
// 事务内幂等查询曾在 REPEATABLE READ 下固定旧读视图，复现「明细实盘已是 7、库存却被调整成 8」。
// 用查询回调把审核停在幂等查询之后、期间提交一次实盘修改，审核必须按新值调整库存。
func TestStocktakeApproveUsesLatestActualQty(t *testing.T) {
	s, db, ctx, inventory := stocktakeFixture(t)
	o, detailID := countedOrder(ctx, t, s, 8)

	arrived := make(chan struct{}, 1)
	release := make(chan struct{})
	var once sync.Once
	if err := db.Callback().Query().After("gorm:query").Register("stocktake:approve_race_idem", func(q *gorm.DB) {
		if q.Statement == nil || q.Statement.Table != "wms_idempotency" {
			return
		}
		once.Do(func() {
			arrived <- struct{}{}
			<-release
		})
	}); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	go func() { errCh <- s.Approve(ctx, o.ID, "test", "appr-key-race") }()
	select {
	case <-arrived:
	case <-time.After(10 * time.Second):
		t.Fatal("approve did not reach idempotency query")
	}
	// 审核已发出幂等查询、尚未锁单读明细时，把实盘数量从 8 改成 7
	if err := s.RecordActual(ctx, o.ID, detailID, 7); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := db.WithContext(ctx).First(inventory, inventory.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.StockQuantity != 7 || inventory.AvailableQty != 7 {
		t.Fatalf("inventory after approve=%+v want stock=7（按最新实盘调整）", inventory)
	}
	var detail model.StocktakeDetail
	if err := db.WithContext(ctx).Where("order_id = ?", o.ID).First(&detail).Error; err != nil {
		t.Fatal(err)
	}
	if detail.ActualQty == nil || *detail.ActualQty != 7 {
		t.Fatalf("actual qty=%v want=7", detail.ActualQty)
	}
}

// TestStocktakeApproveWithKeyUnderSmallPool 连接池上限 1：不带 key 审核、带 key 审核
// 及其同 key 重试都必须完成。幂等预查询若放在事务回调内，事务占住唯一连接后还要
// 申请第二条连接，请求会一直等待直到取消/超时。
func TestStocktakeApproveWithKeyUnderSmallPool(t *testing.T) {
	s, db, ctx, _ := stocktakeFixture(t)
	if sqlDB, err := db.DB(); err != nil {
		t.Fatal(err)
	} else {
		sqlDB.SetMaxOpenConns(1)
	}
	noKey, _ := countedOrder(ctx, t, s, 8)
	if err := s.Approve(ctx, noKey.ID, "test", ""); err != nil {
		t.Fatalf("approve without key under pool=1: %v", err)
	}
	withKey, _ := countedOrder(ctx, t, s, 7)
	if err := s.Approve(ctx, withKey.ID, "test", "appr-small-pool"); err != nil {
		t.Fatalf("approve with key under pool=1: %v", err)
	}
	if err := s.Approve(ctx, withKey.ID, "test", "appr-small-pool"); err != nil {
		t.Fatalf("replay under pool=1: %v", err)
	}
}
