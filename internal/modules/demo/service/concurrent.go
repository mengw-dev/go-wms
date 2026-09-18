package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	inbounddto "gowms/internal/modules/inbound/dto"
	invmodel "gowms/internal/modules/inventory/model"
	outbounddto "gowms/internal/modules/outbound/dto"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

type ConcurrentResult struct {
	Concurrency    int            `json:"concurrency"`
	QtyPerOrder    int            `json:"qty_per_order"`
	TotalDemand    int            `json:"total_demand"`
	Success        int            `json:"success"`
	Failed         int            `json:"failed"`
	DurationMs     int64          `json:"duration_ms"`
	PickTaskCount  int            `json:"pick_task_count"`
	StockTotal     int64          `json:"stock_total"`
	AvailableTotal int64          `json:"available_total"`
	AllocatedTotal int64          `json:"allocated_total"`
	NegativeRows   int64          `json:"negative_rows"`
	TestFocus      string         `json:"test_focus"`
	Summary        string         `json:"summary"`
	Steps          []ScenarioStep `json:"steps"`
}

type PickingResult struct {
	Workers           int            `json:"workers"`
	Contenders        int            `json:"contenders"`
	TaskCount         int            `json:"task_count"`
	TotalTarget       int            `json:"total_target"`
	WorkerSuccess     int            `json:"worker_success"`
	WorkerRejected    int            `json:"worker_rejected"`
	ContenderSuccess  int            `json:"contender_success"`
	ContenderRejected int            `json:"contender_rejected"`
	FinalPicked       int            `json:"final_picked"`
	CompletedTasks    int            `json:"completed_tasks"`
	ShippedOrders     int            `json:"shipped_orders"`
	DurationMs        int64          `json:"duration_ms"`
	StockTotal        int64          `json:"stock_total"`
	AvailableTotal    int64          `json:"available_total"`
	AllocatedTotal    int64          `json:"allocated_total"`
	NegativeRows      int64          `json:"negative_rows"`
	Summary           string         `json:"summary"`
	Steps             []ScenarioStep `json:"steps"`
}

func (s *Service) RunConcurrentAllocation(ctx context.Context, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if concurrency <= 0 {
		concurrency = 20
	}
	if concurrency > 100 {
		concurrency = 100
	}
	if qtyPerOrder <= 0 {
		qtyPerOrder = 1
	}
	if qtyPerOrder > 10 {
		qtyPerOrder = 10
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()

	refs, err := s.loadDemoBaseRefs(ctx)
	if err != nil {
		return nil, err
	}
	totalDemand := concurrency * qtyPerOrder
	statsBefore, err := s.concurrentInventoryStats(ctx, refs.SKU.ID)
	if err != nil {
		return nil, err
	}
	if statsBefore.AvailableTotal < int64(totalDemand) {
		return nil, errcode.New(errcode.DemoStockNotEnough.Code, fmt.Sprintf(
			"可用库存不足：当前可用 %d 件，本次并发出库需要 %d 件。请先点击“一键补货入库”完成入库→收货→上架，或减少并发数量",
			statsBefore.AvailableTotal, totalDemand,
		))
	}

	start := time.Now()
	var success atomic.Int64
	var failed atomic.Int64
	errs := make(chan string, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		index := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.prepareConcurrentOutbound(ctx, refs, index, qtyPerOrder); err != nil {
				failed.Add(1)
				select {
				case errs <- err.Error():
				default:
				}
				return
			}
			success.Add(1)
		}()
	}
	wg.Wait()
	close(errs)

	var pickTaskCount int64
	if err := s.db.WithContext(ctx).Model(&taskmodel.Task{}).
		Where("task_type = ? AND status <> ?", taskmodel.TaskPick, taskmodel.TaskCancelled).
		Count(&pickTaskCount).Error; err != nil {
		return nil, err
	}
	stats, err := s.concurrentInventoryStats(ctx, refs.SKU.ID)
	if err != nil {
		return nil, err
	}
	failReasons := make([]string, 0, 3)
	for reason := range errs {
		if len(failReasons) >= 3 {
			break
		}
		failReasons = append(failReasons, reason)
	}
	testFocus := "出库单并发创建/提交/审核、库存行锁、FIFO 分配、事务冲突重试、防超卖"
	steps := []ScenarioStep{
		{Title: "并发模型", Detail: fmt.Sprintf("同时创建并审核 %d 张出库单，每张 %d 件，总需求 %d 件", concurrency, qtyPerOrder, totalDemand)},
		{Title: "测试重点", Detail: testFocus},
		{Title: "审核分配完成", Detail: fmt.Sprintf("%d 张出库单进入 PICKING，生成 %d 个拣货任务", success.Load(), pickTaskCount)},
		{Title: "业务拒绝", Detail: fmt.Sprintf("%d 张因库存锁或库存不足被业务规则拒绝", failed.Load())},
		{Title: "库存快照", Detail: fmt.Sprintf("stock=%d available=%d allocated=%d 负数行=%d", stats.StockTotal, stats.AvailableTotal, stats.AllocatedTotal, stats.NegativeRows)},
		{Title: "下一步", Detail: "点击“PDA 并发拣货”，使用已生成的拣货任务测试逐件扫码、防超拣和任务锁"},
	}
	if len(failReasons) > 0 {
		steps = append(steps, ScenarioStep{Title: "失败样例", Detail: failReasons[0]})
	}
	durationMs := time.Since(start).Milliseconds()
	summary := fmt.Sprintf("并发出库审核分配完成：%d 张 × %d 件，成功 %d，业务拒绝 %d，生成 %d 个拣货任务", concurrency, qtyPerOrder, success.Load(), failed.Load(), pickTaskCount)
	return &ConcurrentResult{
		Concurrency:    concurrency,
		QtyPerOrder:    qtyPerOrder,
		TotalDemand:    totalDemand,
		Success:        int(success.Load()),
		Failed:         int(failed.Load()),
		DurationMs:     durationMs,
		PickTaskCount:  int(pickTaskCount),
		StockTotal:     stats.StockTotal,
		AvailableTotal: stats.AvailableTotal,
		AllocatedTotal: stats.AllocatedTotal,
		NegativeRows:   stats.NegativeRows,
		TestFocus:      testFocus,
		Summary:        summary,
		Steps:          steps,
	}, nil
}

func (s *Service) RunConcurrent(ctx context.Context, sessionID string, concurrency, qtyPerOrder int) (*ConcurrentResult, error) {
	return s.RunConcurrentAllocation(ctx, sessionID, concurrency, qtyPerOrder)
}

// RestockDemo 通过完整入库流程补充演示库存：创建入库单、提交、审核、收货、上架。
// 它不会直接修改库存表，也不会清空当前会话中的单据和任务。
func (s *Service) RestockDemo(ctx context.Context, sessionID string, qty int) (*ScenarioResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if qty <= 0 {
		qty = 500
	}
	if qty > 2000 {
		qty = 2000
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()

	refs, err := s.loadDemoBaseRefs(ctx)
	if err != nil {
		return nil, err
	}
	location, err := s.loadRestockLocation(ctx, refs.Warehouse.ID, refs.SKU.ID)
	if err != nil {
		return nil, err
	}

	operator := s.Username()
	order, err := s.inbound.Create(ctx, &inbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		Remark:      fmt.Sprintf("一键补货入库：为并发演示补充 %d 件库存", qty),
		Details: []inbounddto.OrderDetailItem{{
			SKUID: refs.SKU.ID, ExpectedQty: qty,
		}},
	}, operator)
	if err != nil {
		return nil, err
	}
	if err := s.inbound.Submit(ctx, order.ID); err != nil {
		return nil, err
	}
	if err := s.inbound.Approve(ctx, order.ID, operator); err != nil {
		return nil, err
	}

	detail, err := s.inbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	if len(detail.Details) == 0 {
		return nil, errcode.DemoDataMissing
	}
	batchNo := demoBatchNo()
	if err := s.inbound.Receive(ctx, order.ID, detail.Details[0].ID, &inbounddto.ReceiveReq{
		DetailID: detail.Details[0].ID, Qty: qty, BatchNo: batchNo,
	}, operator); err != nil {
		return nil, err
	}

	detail, err = s.inbound.Get(ctx, order.ID)
	if err != nil {
		return nil, err
	}
	var putawayTaskID int64
	var putawayQty int
	for _, task := range detail.Tasks {
		if task.TaskType == taskmodel.TaskPutaway && task.Status != taskmodel.TaskCompleted && task.DoneQty < task.TargetQty {
			putawayTaskID = task.ID
			putawayQty = task.TargetQty - task.DoneQty
			break
		}
	}
	if putawayTaskID == 0 {
		return nil, errcode.DemoDataMissing
	}
	if err := s.inbound.Putaway(ctx, putawayTaskID, location.ID, putawayQty, operator); err != nil {
		return nil, err
	}

	return &ScenarioResult{
		Name:        "restock",
		Summary:     fmt.Sprintf("一键补货完成：已通过完整入库流程补充 %d 件库存", qty),
		TargetPath:  "/inventory",
		TargetLabel: "查看库存",
		Steps: []ScenarioStep{
			{Title: "创建入库单", Detail: order.OrderNo},
			{Title: "提交并审核", Detail: "状态机推进到 APPROVED，生成收货任务"},
			{Title: "完成收货", Detail: fmt.Sprintf("货品 %s，批次 %s，数量 %d", refs.SKU.Code, batchNo, qty)},
			{Title: "完成上架", Detail: fmt.Sprintf("库位 %s，可用库存增加 %d 件", location.Code, qty)},
			{Title: "下一步", Detail: "点击“并发出库审核分配”，然后执行“PDA 并发拣货”"},
		},
	}, nil
}

func (s *Service) RunConcurrentPicking(ctx context.Context, sessionID string, workers, contenders int) (*PickingResult, error) {
	if err := s.ValidateSession(ctx, sessionID); err != nil {
		return nil, err
	}
	if workers <= 0 {
		workers = 10
	}
	if workers > 100 {
		workers = 100
	}
	if contenders < 0 {
		contenders = 0
	}
	if contenders > 50 {
		contenders = 50
	}

	s.runMu.Lock()
	defer s.runMu.Unlock()

	var tasks []*taskmodel.Task
	if err := s.db.WithContext(ctx).
		Where("task_type = ? AND status IN ?", taskmodel.TaskPick, []taskmodel.TaskStatus{taskmodel.TaskCreated, taskmodel.TaskInProgress}).
		Order("id ASC").
		Limit(200).
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, errcode.DemoPickTaskMissing
	}

	taskIDs := make([]int64, 0, len(tasks))
	orderIDs := make([]int64, 0, len(tasks))
	totalTarget := 0
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		orderIDs = append(orderIDs, task.OrderID)
		totalTarget += task.TargetQty
	}
	skuID := tasks[0].SKUID
	operator := s.Username()
	start := time.Now()

	var workerSuccess, workerRejected, contenderSuccess, contenderRejected atomic.Int64
	var nextTask atomic.Int64

	var workerWG sync.WaitGroup
	for i := 0; i < workers; i++ {
		workerWG.Add(1)
		go func() {
			defer workerWG.Done()
			for {
				index := int(nextTask.Add(1)) - 1
				if index >= len(tasks) {
					return
				}
				task := tasks[index]
				pickRemaining(ctx, s, task.ID, task.TargetQty-task.DoneQty, operator, &workerSuccess, &workerRejected)
			}
		}()
	}

	var contenderWG sync.WaitGroup
	for i := 0; i < contenders; i++ {
		contenderWG.Add(1)
		go func(seed int64) {
			defer contenderWG.Done()
			rng := rand.New(rand.NewSource(time.Now().UnixNano() + seed))
			attempts := len(tasks) * 2
			for j := 0; j < attempts; j++ {
				task := tasks[rng.Intn(len(tasks))]
				if err := s.outbound.Pick(ctx, task.ID, 1, operator, nil); err == nil {
					contenderSuccess.Add(1)
				} else {
					contenderRejected.Add(1)
				}
				time.Sleep(2 * time.Millisecond)
			}
		}(int64(i))
	}
	workerWG.Wait()
	contenderWG.Wait()

	// 兜底补齐前必须重新读库：内存里的 DoneQty 是并发开始前的旧值，
	// 直接拿它算剩余量，会把拣货员已经拣满的任务再补一遍，凭空产生一批"并发拒绝"。
	var pendingTasks []*taskmodel.Task
	if err := s.db.WithContext(ctx).Where("id IN ?", taskIDs).Order("id ASC").Find(&pendingTasks).Error; err != nil {
		return nil, err
	}
	for _, task := range pendingTasks {
		if task.Status == taskmodel.TaskCompleted {
			continue
		}
		if remaining := task.TargetQty - task.DoneQty; remaining > 0 {
			pickRemaining(ctx, s, task.ID, remaining, operator, &workerSuccess, &workerRejected)
		}
	}

	var finalTasks []*taskmodel.Task
	if err := s.db.WithContext(ctx).Where("id IN ?", taskIDs).Order("id ASC").Find(&finalTasks).Error; err != nil {
		return nil, err
	}
	finalPicked := 0
	completedTasks := 0
	for _, task := range finalTasks {
		finalPicked += task.DoneQty
		if task.Status == taskmodel.TaskCompleted {
			completedTasks++
		}
	}
	var shippedOrders int64
	if len(orderIDs) > 0 {
		if err := s.db.WithContext(ctx).Model(&outboundmodel.ShipmentOrder{}).
			Where("id IN ? AND status = ?", orderIDs, outboundmodel.OrderShipped).
			Count(&shippedOrders).Error; err != nil {
			return nil, err
		}
	}
	stats, err := s.concurrentInventoryStats(ctx, skuID)
	if err != nil {
		return nil, err
	}
	durationMs := time.Since(start).Milliseconds()
	summary := fmt.Sprintf("PDA 并发拣货完成：%d 个任务，%d 个拣货员，%d 个抢单者，最终拣货 %d/%d，正常扫码 %d 次，并发拒绝 %d 次",
		len(tasks), workers, contenders, finalPicked, totalTarget, workerSuccess.Load(), workerRejected.Load()+contenderRejected.Load())
	steps := []ScenarioStep{
		{Title: "PDA 并发模型", Detail: fmt.Sprintf("%d 个拣货员逐件扫码，%d 个抢单者模拟重复扫码/误派", workers, contenders)},
		{Title: "测试重点", Detail: "任务行锁、分配行版本、逐件扫码、同一任务抢拣、防超拣、防负库存"},
		{Title: "拣货结果", Detail: fmt.Sprintf("正常扫码成功 %d 次，并发/重复扫码拒绝 %d 次", workerSuccess.Load(), workerRejected.Load()+contenderRejected.Load())},
		{Title: "终态校验", Detail: fmt.Sprintf("任务完成 %d/%d，订单已发货 %d 张，最终拣货 %d/%d", completedTasks, len(finalTasks), shippedOrders, finalPicked, totalTarget)},
		{Title: "库存快照", Detail: fmt.Sprintf("stock=%d available=%d allocated=%d 负数行=%d", stats.StockTotal, stats.AvailableTotal, stats.AllocatedTotal, stats.NegativeRows)},
	}
	return &PickingResult{
		Workers:           workers,
		Contenders:        contenders,
		TaskCount:         len(tasks),
		TotalTarget:       totalTarget,
		WorkerSuccess:     int(workerSuccess.Load()),
		WorkerRejected:    int(workerRejected.Load()),
		ContenderSuccess:  int(contenderSuccess.Load()),
		ContenderRejected: int(contenderRejected.Load()),
		FinalPicked:       finalPicked,
		CompletedTasks:    completedTasks,
		ShippedOrders:     int(shippedOrders),
		DurationMs:        durationMs,
		StockTotal:        stats.StockTotal,
		AvailableTotal:    stats.AvailableTotal,
		AllocatedTotal:    stats.AllocatedTotal,
		NegativeRows:      stats.NegativeRows,
		Summary:           summary,
		Steps:             steps,
	}, nil
}

func (s *Service) prepareConcurrentOutbound(ctx context.Context, refs *demoRefs, index, qtyPerOrder int) error {
	operator := s.Username()
	order, err := s.outbound.Create(ctx, &outbounddto.CreateOrderReq{
		WarehouseID: refs.Warehouse.ID,
		BizOrderNo:  demoBizOrderNo(index + 1),
		Remark:      "并发审核分配：库存锁、FIFO 与防超卖",
		Details:     []outbounddto.OrderDetailItem{{SKUID: refs.SKU.ID, ExpectedQty: qtyPerOrder}},
	}, operator)
	if err != nil {
		return err
	}
	if err := s.outbound.Submit(ctx, order.ID); err != nil {
		return err
	}
	return s.outbound.Approve(ctx, order.ID, operator)
}

func (s *Service) loadRestockLocation(ctx context.Context, warehouseID, skuID int64) (*basicmodel.Location, error) {
	var location basicmodel.Location
	err := s.db.WithContext(ctx).
		Table("wms_location AS l").
		Select("l.*").
		Joins("JOIN wms_inventory AS i ON i.location_id = l.id").
		Where("l.warehouse_id = ? AND l.status <> ? AND i.sku_id = ?", warehouseID, basicmodel.LocationStatusDisabled, skuID).
		Order("i.stock_in_time ASC, l.code ASC").
		First(&location).Error
	if err == nil {
		return &location, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := s.db.WithContext(ctx).
		Where("warehouse_id = ? AND status <> ?", warehouseID, basicmodel.LocationStatusDisabled).
		Order("code ASC").
		First(&location).Error; err != nil {
		return nil, errcode.DemoDataMissing
	}
	return &location, nil
}

func pickRemaining(ctx context.Context, s *Service, taskID int64, remaining int, operator string, success, rejected *atomic.Int64) {
	consecutiveRejects := 0
	for i := 0; i < remaining; i++ {
		if err := s.outbound.Pick(ctx, taskID, 1, operator, nil); err != nil {
			rejected.Add(1)
			consecutiveRejects++
			if consecutiveRejects >= 3 {
				return
			}
			time.Sleep(2 * time.Millisecond)
			continue
		}
		success.Add(1)
		consecutiveRejects = 0
	}
}

type concurrentInventoryStats struct {
	StockTotal     int64
	AvailableTotal int64
	AllocatedTotal int64
	NegativeRows   int64
}

func (s *Service) concurrentInventoryStats(ctx context.Context, skuID int64) (*concurrentInventoryStats, error) {
	var stats concurrentInventoryStats
	if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Where("sku_id = ?", skuID).
		Select("COALESCE(SUM(stock_quantity), 0) AS stock_total, COALESCE(SUM(available_quantity), 0) AS available_total, COALESCE(SUM(allocated_quantity), 0) AS allocated_total").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Where("sku_id = ? AND (stock_quantity < 0 OR available_quantity < 0 OR allocated_quantity < 0)", skuID).
		Count(&stats.NegativeRows).Error; err != nil {
		return nil, err
	}
	return &stats, nil
}
