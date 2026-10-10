package app

// 同 key 并发下的幂等回放加固测试（真实 MySQL + gorm 回调控制交错）：
//   - 后到者的事务快照读不到先到者提交的幂等记录，拿锁后又因旧状态（任务已完成/数量变化）被业务拒绝；
//   - 业务事务回滚后必须用新读核对已提交的幂等记录，命中则回放首次成功，
//     不能把后到者误报为「新业务操作失败」诱导客户端换 key；指纹不同必须返回准确的 409。
// 另覆盖：幂等记录插入失败时业务整笔回滚、回放快照字段校验、租约过期后的回放、租户与 scope 隔离。

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	invmodel "gowms/internal/modules/inventory/model"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	outservice "gowms/internal/modules/outbound/service"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

// strictScan 构造与任务一致的 PDA 严格扫码。
func strictScan(task *taskmodel.Task) *outservice.PickScan {
	return &outservice.PickScan{Strict: true, LocationCode: task.LocationCode, BatchNo: task.BatchNo}
}

// signalIdempotencyRead 在 wms_idempotency 的 SELECT（事务内幂等快路径）下发前关闭一次信号。
// 在「先到者已持任务行锁且未提交」之后注册：先到者已完成查询不会再触发，
// 信号关闭即代表后到者的查询已执行（其事务快照中必然查不到先到者未提交的记录）。
func signalIdempotencyRead(t *testing.T, db *gorm.DB, name string, ch chan struct{}) {
	t.Helper()
	var once sync.Once
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(query *gorm.DB) {
		if query.Statement == nil || query.Statement.Table != "wms_idempotency" {
			return
		}
		if _, ok := query.Statement.Dest.(*idempotency.Record); !ok {
			return
		}
		once.Do(func() { close(ch) })
	}); err != nil {
		t.Fatal(err)
	}
}

// TestSameKeyConcurrentPartialPick 同 key 并发部分拣货：两个请求都查不到记录、
// 竞争同一任务行，业务累计一次；后到者走唯一键冲突→重试→回放首次成功。
func TestSameKeyConcurrentPartialPick(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-RACE-PART", 2)
	const key = "idem-race-part"
	scan := strictScan(task)

	gateA := newTaskUpdateGate(t, stack.db, "race:idem_part_a")
	aDone := make(chan error, 1)
	go func() { _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", key); aDone <- err }()
	waitClosed(t, "A holds task row", gateA.arrived)

	bRead := make(chan struct{})
	signalIdempotencyRead(t, stack.db, "race:idem_part_b_read", bRead)
	var bResult *outdto.PickResult
	var bErr error
	bDone := make(chan struct{})
	go func() {
		bResult, bErr = stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", key)
		close(bDone)
	}()
	waitClosed(t, "B has read idempotency record", bRead)
	close(gateA.release)

	if err := waitError(t, "A", aDone); err != nil {
		t.Fatalf("A should succeed: %v", err)
	}
	waitClosed(t, "B", bDone)
	if bErr != nil {
		t.Fatalf("B should replay first success, got: %v", bErr)
	}
	if bResult == nil || bResult.DoneQty != 1 {
		t.Fatalf("B result=%+v", bResult)
	}

	// 业务只累计一次：任务/分配行都不重复推进，库存仅停留在分配占位。
	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 1 || gotTask.Status != taskmodel.TaskInProgress {
		t.Fatalf("task done=%d status=%s", gotTask.DoneQty, gotTask.Status)
	}
	if gotAlloc := stack.reloadAllocation(t, task.AllocationID); gotAlloc.PickedQty != 1 || gotAlloc.Status != outmodel.AllocAllocated {
		t.Fatalf("allocation picked=%d status=%s", gotAlloc.PickedQty, gotAlloc.Status)
	}
	if inv := stack.inventory(t); inv.StockQuantity != raceSeedQty || inv.AvailableQty != raceSeedQty-2 || inv.AllocatedQty != 2 {
		t.Fatalf("inventory drift: %+v", inv)
	}
}

// TestSameKeyConcurrentPickFullRemaining 同 key 并发完成全部剩余量：
// 先到者拣完并关闭任务，后到者拿锁后看到任务已完成——不能报 TaskStatusWrong，
// 必须用新读核对已提交记录并回放首次成功。
func TestSameKeyConcurrentPickFullRemaining(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-RACE-FULL", 1)
	const key = "idem-race-full"
	scan := strictScan(task)

	gateA := newTaskUpdateGate(t, stack.db, "race:idem_full_a")
	aDone := make(chan error, 1)
	go func() { _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", key); aDone <- err }()
	waitClosed(t, "A holds task row", gateA.arrived)

	bRead := make(chan struct{})
	signalIdempotencyRead(t, stack.db, "race:idem_full_b_read", bRead)
	var bResult *outdto.PickResult
	var bErr error
	bDone := make(chan struct{})
	go func() {
		bResult, bErr = stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", key)
		close(bDone)
	}()
	waitClosed(t, "B has read idempotency record", bRead)
	close(gateA.release)

	if err := waitError(t, "A", aDone); err != nil {
		t.Fatalf("A should succeed: %v", err)
	}
	waitClosed(t, "B", bDone)
	if bErr != nil {
		t.Fatalf("B should replay first success instead of business failure, got: %v", bErr)
	}
	if bResult == nil || bResult.DoneQty != 1 {
		t.Fatalf("B result=%+v", bResult)
	}

	// 唯一的真实拣货来自先到者：任务完成、分配行拣满、库存完成发货扣减。
	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 1 || gotTask.Status != taskmodel.TaskCompleted {
		t.Fatalf("task done=%d status=%s", gotTask.DoneQty, gotTask.Status)
	}
	if gotAlloc := stack.reloadAllocation(t, task.AllocationID); gotAlloc.PickedQty != 1 || gotAlloc.Status != outmodel.AllocPicked {
		t.Fatalf("allocation picked=%d status=%s", gotAlloc.PickedQty, gotAlloc.Status)
	}
	if inv := stack.inventory(t); inv.StockQuantity != raceSeedQty-1 || inv.AvailableQty != raceSeedQty-1 || inv.AllocatedQty != 0 {
		t.Fatalf("inventory drift: %+v", inv)
	}
}

// TestSameKeyConcurrentDifferentContent 同 key 不同内容并发：
// 先到者整单拣完，后到者同 key 携带不同数量——即使并发竞争，
// 后到者也必须拿到 409 内容冲突，而不是被数量/状态类业务错误掩盖。
func TestSameKeyConcurrentDifferentContent(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-RACE-DIFF", 2)
	const key = "idem-race-diff"
	scan := strictScan(task)

	gateA := newTaskUpdateGate(t, stack.db, "race:idem_diff_a")
	aDone := make(chan error, 1)
	go func() { _, err := stack.out.Pick(stack.ctx, task.ID, 2, "picker", scan, "", key); aDone <- err }()
	waitClosed(t, "A holds task row", gateA.arrived)

	bRead := make(chan struct{})
	signalIdempotencyRead(t, stack.db, "race:idem_diff_b_read", bRead)
	var bResult *outdto.PickResult
	var bErr error
	bDone := make(chan struct{})
	go func() {
		bResult, bErr = stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", key)
		close(bDone)
	}()
	waitClosed(t, "B has read idempotency record", bRead)
	close(gateA.release)

	if err := waitError(t, "A", aDone); err != nil {
		t.Fatalf("A should succeed: %v", err)
	}
	waitClosed(t, "B", bDone)
	requireCode(t, "B reused key with different content", bErr, errcode.IdempotencyKeyReused)
	if bResult != nil {
		t.Fatalf("B must not produce a result on conflict: %+v", bResult)
	}

	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 2 || gotTask.Status != taskmodel.TaskCompleted {
		t.Fatalf("task done=%d status=%s", gotTask.DoneQty, gotTask.Status)
	}
}

// TestSameKeyConcurrentClaimKeepsFirstToken 同 key 并发领取：
// 两个请求都成功且返回同一凭证，数据库终态与该凭证一致，不发生意外轮换。
func TestSameKeyConcurrentClaimKeepsFirstToken(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-RACE-CLAIM", 2)
	const key = "idem-race-claim"

	barrier := make(chan struct{})
	var results [2]*outdto.ClaimResult
	var errs [2]error
	var wg sync.WaitGroup
	wg.Add(2)
	for i := range 2 {
		go func(idx int) {
			defer wg.Done()
			<-barrier
			results[idx], errs[idx] = stack.out.ClaimPickTask(stack.ctx, task.ID, "picker", key)
		}(i)
	}
	close(barrier)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	waitClosed(t, "claim pair", done)

	for i := range 2 {
		if errs[i] != nil {
			t.Fatalf("claim %d: %v", i, errs[i])
		}
		if results[i] == nil || results[i].ClaimToken == "" {
			t.Fatalf("claim %d returned empty token", i)
		}
	}
	if results[0].ClaimToken != results[1].ClaimToken {
		t.Fatalf("tokens diverged: %s vs %s", results[0].ClaimToken, results[1].ClaimToken)
	}
	gotTask := stack.reloadTask(t, task.ID)
	if gotTask.ClaimToken != results[0].ClaimToken {
		t.Fatalf("db claim_token=%s want=%s", gotTask.ClaimToken, results[0].ClaimToken)
	}
}

// TestPickIdempotencyInsertFailureRollsBack 幂等记录插入失败时，
// 任务、订单、分配行与库存必须与业务写入一起整体回滚，不留下半成功状态。
func TestPickIdempotencyInsertFailureRollsBack(t *testing.T) {
	stack := newPickRaceStack(t)
	order, task := stack.newPickOrder(t, "IDEM-RACE-INSFAIL", 2)
	scan := strictScan(task)

	countTrans := func() int64 {
		var n int64
		if err := stack.db.WithContext(stack.ctx).Model(&invmodel.InventoryTrans{}).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	beforeTrans := countTrans()

	var once sync.Once
	if err := stack.db.Callback().Create().Before("gorm:create").Register("race:idem_insert_fail", func(q *gorm.DB) {
		if q.Statement == nil || q.Statement.Table != "wms_idempotency" {
			return
		}
		once.Do(func() { _ = q.AddError(errors.New("injected idempotency insert failure")) })
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", "idem-race-insfail"); errcode.From(err).Code != errcode.Internal.Code {
		t.Fatalf("pick err=%v", err)
	}

	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 0 || gotTask.Status != taskmodel.TaskCreated {
		t.Fatalf("task changed: done=%d status=%s", gotTask.DoneQty, gotTask.Status)
	}
	if gotOrder := stack.reloadOrder(t, order.ID); gotOrder.PickedQty != 0 || gotOrder.Status != outmodel.OrderPicking {
		t.Fatalf("order changed: picked=%d status=%s", gotOrder.PickedQty, gotOrder.Status)
	}
	if gotAlloc := stack.reloadAllocation(t, task.AllocationID); gotAlloc.PickedQty != 0 || gotAlloc.Status != outmodel.AllocAllocated {
		t.Fatalf("allocation changed: picked=%d status=%s", gotAlloc.PickedQty, gotAlloc.Status)
	}
	if inv := stack.inventory(t); inv.StockQuantity != raceSeedQty || inv.AvailableQty != raceSeedQty-2 || inv.AllocatedQty != 2 {
		t.Fatalf("inventory changed: %+v", inv)
	}
	// 拣满扣减才写流水：插入失败后流水数必须与拣货前一致。
	if afterTrans := countTrans(); afterTrans != beforeTrans {
		t.Fatalf("inventory trans count changed: before=%d after=%d", beforeTrans, afterTrans)
	}
	var idemCount int64
	if err := stack.db.WithContext(stack.ctx).Model(&idempotency.Record{}).Count(&idemCount).Error; err != nil {
		t.Fatal(err)
	}
	if idemCount != 0 {
		t.Fatalf("idempotency records=%d want=0", idemCount)
	}
}

// TestReplayRejectsMalformedPickSnapshot 「能解析成 JSON」不等于「有效成功结果」：
// 空结构、null、缺字段、字段类型错误都必须按内部错误处理，不能用零值伪装成功回放；
// 数量 0 是合法值，不能被当成缺失。
func TestReplayRejectsMalformedPickSnapshot(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-SHAPE-PICK", 2)
	scan := strictScan(task)

	first, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", "shape-pick-1")
	if err != nil || first.DoneQty != 1 {
		t.Fatalf("first pick: result=%+v err=%v", first, err)
	}

	badShapes := []string{
		`null`,
		`{}`,
		`{"task_status":"IN_PROGRESS","order_status":"PICKING"}`,
		`{"task_status":"IN_PROGRESS","done_qty":"1","remaining_qty":1,"order_status":"PICKING"}`,
		`{"task_status":1,"done_qty":1,"remaining_qty":1,"order_status":"PICKING"}`,
		`{"task_status":"IN_PROGRESS","done_qty":null,"remaining_qty":1,"order_status":"PICKING"}`,
	}
	for _, shape := range badShapes {
		if err := stack.db.WithContext(stack.ctx).Model(&idempotency.Record{}).
			Where("idempotency_key = ?", "shape-pick-1").Update("result_json", shape).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", "shape-pick-1"); errcode.From(err).Code != errcode.Internal.Code {
			t.Fatalf("shape %s should be internal error, got: %v", shape, err)
		}
	}
	// 合法形状（含 0 值数量）应正常回放，而不是被判为损坏。
	good := `{"task_status":"IN_PROGRESS","done_qty":0,"remaining_qty":2,"order_status":"PICKING"}`
	if err := stack.db.WithContext(stack.ctx).Model(&idempotency.Record{}).
		Where("idempotency_key = ?", "shape-pick-1").Update("result_json", good).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", "shape-pick-1")
	if err != nil || replay.DoneQty != 0 || replay.RemainingQty != 2 {
		t.Fatalf("valid-zero replay: result=%+v err=%v", replay, err)
	}
	// 全程真实业务只执行一次
	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 1 {
		t.Fatalf("task done=%d want=1", gotTask.DoneQty)
	}
}

// TestReplayRejectsMalformedClaimSnapshot 领取回放同样需要字段校验：
// 缺失凭证或租约信息的记录属损坏，返回内部错误，而不是回放一个空凭证。
func TestReplayRejectsMalformedClaimSnapshot(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-SHAPE-CLAIM", 2)

	first, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "picker", "shape-claim-1")
	if err != nil || first.ClaimToken == "" {
		t.Fatalf("first claim: result=%+v err=%v", first, err)
	}

	badShapes := []string{
		`null`,
		`{}`,
		`{"claim_token":""}`,
		`{"claim_token":"t1"}`,
		`{"claim_token":"t1","lease_expire_at":"not-a-time"}`,
	}
	for _, shape := range badShapes {
		if err := stack.db.WithContext(stack.ctx).Model(&idempotency.Record{}).
			Where("idempotency_key = ?", "shape-claim-1").Update("result_json", shape).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "picker", "shape-claim-1"); errcode.From(err).Code != errcode.Internal.Code {
			t.Fatalf("shape %s should be internal error, got: %v", shape, err)
		}
	}
}

// TestReplayIgnoresExpiredLease 租约过期后，已成功请求的原 key 重试仍回放首次成功，
// 不能因当前凭证过期而重新执行或误拒。
func TestReplayIgnoresExpiredLease(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-LEASE", 2)

	claim, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "picker", "lease-claim-1")
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	scan := strictScan(task)
	first, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, claim.ClaimToken, "lease-pick-1")
	if err != nil || first.DoneQty != 1 {
		t.Fatalf("first pick: result=%+v err=%v", first, err)
	}
	if err := stack.db.WithContext(stack.ctx).Model(&taskmodel.Task{}).
		Where("id = ?", task.ID).Update("lease_expire_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	replay, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, claim.ClaimToken, "lease-pick-1")
	if err != nil || replay.DoneQty != first.DoneQty {
		t.Fatalf("expired-lease replay: result=%+v err=%v", replay, err)
	}
	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 1 {
		t.Fatalf("task done=%d want=1", gotTask.DoneQty)
	}
}

// TestIdempotencyTenantAndScopeIsolation 幂等键按 (tenant_id, scope) 隔离：
// 其他租户、其他 scope、平台租户 0 的同 key 记录都不影响本租户的正常新操作。
func TestIdempotencyTenantAndScopeIsolation(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-ISOLATE", 2)
	scan := strictScan(task)

	fixtures := []*idempotency.Record{
		{TenantID: raceTenantID + 1, Scope: "outbound.pick", IdempotencyKey: "iso-key"},
		{TenantID: raceTenantID, Scope: "other.scope", IdempotencyKey: "iso-key"},
		{TenantID: 0, Scope: "outbound.pick", IdempotencyKey: "iso-key"},
	}
	for _, fixture := range fixtures {
		fixture.ID = snowflake.Next()
		fixture.RequestHash = "other-hash"
		fixture.ObjectID = task.ID
		fixture.ResultJSON = `{"task_status":"IN_PROGRESS","done_qty":1,"remaining_qty":1,"order_status":"PICKING"}`
		fixture.CreatedAt = time.Now()
		// 平台旁路上下文写入跨租户/跨 scope fixture：正租户上下文不允许显式写其他 tenant_id。
		if err := stack.db.WithContext(tenant.WithTenant(context.Background(), 0)).Create(fixture).Error; err != nil {
			t.Fatal(err)
		}
	}

	// 本租户 + 本 scope 的 key 未命中：按新操作正常执行
	first, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", scan, "", "iso-key")
	if err != nil || first.DoneQty != 1 {
		t.Fatalf("isolated pick: result=%+v err=%v", first, err)
	}
	if gotTask := stack.reloadTask(t, task.ID); gotTask.DoneQty != 1 {
		t.Fatalf("task done=%d want=1", gotTask.DoneQty)
	}
}
