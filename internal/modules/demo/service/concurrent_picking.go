package service

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

const maxPickingDuplicateChecks = 5

type PickingInventoryEvidence struct {
	TransType       invmodel.TransType `json:"trans_type"`
	QuantityChange  int                `json:"quantity_change"`
	BeforeQuantity  int                `json:"before_quantity"`
	AfterQuantity   int                `json:"after_quantity"`
	AvailableBefore int                `json:"available_before"`
	AvailableAfter  int                `json:"available_after"`
	OrderNo         string             `json:"order_no"`
	TaskNo          string             `json:"task_no"`
	CreatedAt       time.Time          `json:"created_at"`
}

type PickingResult struct {
	Workers                        int                        `json:"workers"`
	Contenders                     int                        `json:"contenders"`
	TaskCount                      int                        `json:"task_count"`
	TotalTarget                    int                        `json:"total_target"`
	ConcurrentScanAttempts         int                        `json:"concurrent_scan_attempts"`
	ConcurrentSuccess              int                        `json:"concurrent_success"`
	CompetitionRejected            int                        `json:"competition_rejected"`
	StillIncompleteAfterConcurrent int                        `json:"still_incomplete_after_concurrent"`
	WorkerSuccess                  int                        `json:"worker_success"`
	WorkerRejected                 int                        `json:"worker_rejected"`
	ContenderSuccess               int                        `json:"contender_success"`
	ContenderRejected              int                        `json:"contender_rejected"`
	CleanupRemainingTasks          int                        `json:"cleanup_remaining_tasks"`
	CleanupPickedQuantity          int                        `json:"cleanup_picked_quantity"`
	CleanupRejected                int                        `json:"cleanup_rejected"`
	DuplicateScanAttempts          int                        `json:"duplicate_scan_attempts"`
	DuplicateScanRejected          int                        `json:"duplicate_scan_rejected"`
	DuplicateScanSuccess           int                        `json:"duplicate_scan_success"`
	FinalPicked                    int                        `json:"final_picked"`
	CompletedTasks                 int                        `json:"completed_tasks"`
	ShippedOrders                  int                        `json:"shipped_orders"`
	DurationMs                     int64                      `json:"duration_ms"`
	StockTotal                     int64                      `json:"stock_total"`
	AvailableTotal                 int64                      `json:"available_total"`
	AllocatedTotal                 int64                      `json:"allocated_total"`
	NegativeRows                   int64                      `json:"negative_rows"`
	InvariantOK                    bool                       `json:"invariant_ok"`
	InvariantMessage               string                     `json:"invariant_message"`
	InventoryTrans                 []PickingInventoryEvidence `json:"inventory_trans"`
	Summary                        string                     `json:"summary"`
	Steps                          []ScenarioStep             `json:"steps"`
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

	runCtx, finish, err := s.beginTenantRun(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	defer finish()
	ctx = runCtx

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
	operator := s.Username(ctx)
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
			// #nosec G404 -- 仅用于演示并发测试的任务选择，不用于安全用途。
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

	var afterConcurrent []*taskmodel.Task
	if err := s.db.WithContext(ctx).Where("id IN ?", taskIDs).Order("id ASC").Find(&afterConcurrent).Error; err != nil {
		return nil, err
	}
	stillIncomplete := 0
	for _, task := range afterConcurrent {
		if task.Status != taskmodel.TaskCompleted && task.DoneQty < task.TargetQty {
			stillIncomplete++
		}
	}

	// 收尾阶段是明确独立、顺序执行的补偿过程；不能把它并入并发扫码结果。
	var cleanupSuccess, cleanupRejected atomic.Int64
	cleanupRemainingTasks := 0
	for _, task := range afterConcurrent {
		if task.Status == taskmodel.TaskCompleted {
			continue
		}
		remaining := task.TargetQty - task.DoneQty
		if remaining <= 0 {
			continue
		}
		cleanupRemainingTasks++
		pickRemaining(ctx, s, task.ID, remaining, operator, &cleanupSuccess, &cleanupRejected)
	}

	var afterCleanup []*taskmodel.Task
	if err := s.db.WithContext(ctx).Where("id IN ?", taskIDs).Order("id ASC").Find(&afterCleanup).Error; err != nil {
		return nil, err
	}
	completedTaskIDs := make([]int64, 0, len(afterCleanup))
	for _, task := range afterCleanup {
		if task.Status == taskmodel.TaskCompleted {
			completedTaskIDs = append(completedTaskIDs, task.ID)
		}
	}

	// 重复扫码单独成相，专门验证已完成任务再次扫码会被真实业务规则拒绝。
	var duplicateAttempts, duplicateRejected, duplicateSuccess int
	if len(completedTaskIDs) > 0 {
		limit := maxPickingDuplicateChecks
		if len(completedTaskIDs) < limit {
			limit = len(completedTaskIDs)
		}
		for i := 0; i < limit; i++ {
			duplicateAttempts++
			if err := s.outbound.Pick(ctx, completedTaskIDs[i], 1, operator, nil); err == nil {
				duplicateSuccess++
			} else {
				duplicateRejected++
			}
		}
	}

	var finalTasks []*taskmodel.Task
	if err := s.db.WithContext(ctx).Where("id IN ?", taskIDs).Order("id ASC").Find(&finalTasks).Error; err != nil {
		return nil, err
	}
	finalPicked := 0
	completedTasks := 0
	taskNos := make([]string, 0, len(finalTasks))
	for _, task := range finalTasks {
		finalPicked += task.DoneQty
		taskNos = append(taskNos, task.TaskNo)
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

	var transRows []*invmodel.InventoryTrans
	if len(taskNos) > 0 {
		if err := s.db.WithContext(ctx).
			Where("task_no IN ? AND trans_type = ?", taskNos, invmodel.TransShip).
			Order("created_at ASC").
			Find(&transRows).Error; err != nil {
			return nil, err
		}
	}
	inventoryTrans := make([]PickingInventoryEvidence, 0, len(transRows))
	for _, trans := range transRows {
		inventoryTrans = append(inventoryTrans, PickingInventoryEvidence{
			TransType:       trans.TransType,
			QuantityChange:  trans.QuantityChange,
			BeforeQuantity:  trans.BeforeQuantity,
			AfterQuantity:   trans.AfterQuantity,
			AvailableBefore: trans.AvailableBefore,
			AvailableAfter:  trans.AvailableAfter,
			OrderNo:         trans.OrderNo,
			TaskNo:          trans.TaskNo,
			CreatedAt:       trans.CreatedAt,
		})
	}

	stats, err := s.concurrentInventoryStats(ctx, skuID)
	if err != nil {
		return nil, err
	}
	invariantOK := concurrentInventoryInvariantOK(stats) && duplicateSuccess == 0
	invariantMessage := "库存三数量公式成立，且已完成任务重复扫码均被拒绝"
	if !invariantOK {
		invariantMessage = fmt.Sprintf(
			"不变量异常：stock=%d available=%d allocated=%d，负数行=%d，重复扫码成功=%d",
			stats.StockTotal, stats.AvailableTotal, stats.AllocatedTotal, stats.NegativeRows, duplicateSuccess,
		)
	}
	durationMs := time.Since(start).Milliseconds()
	concurrentSuccess := int(workerSuccess.Load() + contenderSuccess.Load())
	competitionRejected := int(workerRejected.Load() + contenderRejected.Load())
	summary := fmt.Sprintf(
		"模拟 PDA 并发拣货：并发阶段成功扫码 %d 次、竞争拒绝 %d 次，结束后仍有 %d 个任务未完成；收尾阶段顺序补齐 %d 件，最终完成 %d/%d 个任务",
		concurrentSuccess, competitionRejected, stillIncomplete, cleanupSuccess.Load(), completedTasks, len(finalTasks),
	)
	steps := []ScenarioStep{
		{Title: "并发阶段", Detail: fmt.Sprintf("%d 个并发扫码请求，%d 个抢单请求；成功 %d，拒绝 %d", workers, contenders, concurrentSuccess, competitionRejected), Status: ScenarioStepCompleted},
		{Title: "并发后剩余", Detail: fmt.Sprintf("仍有 %d 个任务未完成，总目标 %d 件", stillIncomplete, totalTarget), Status: ScenarioStepCompleted},
		{Title: "收尾阶段", Detail: fmt.Sprintf("顺序处理剩余 %d 个任务，补齐 %d 件，收尾拒绝 %d 次", cleanupRemainingTasks, cleanupSuccess.Load(), cleanupRejected.Load()), Status: ScenarioStepCompleted},
		{Title: "重复扫码验证", Detail: fmt.Sprintf("对已完成任务发起 %d 次重复扫码，真实拒绝 %d 次，异常成功 %d 次", duplicateAttempts, duplicateRejected, duplicateSuccess), Status: ScenarioStepCompleted},
		{Title: "最终业务状态", Detail: fmt.Sprintf("PICK 完成 %d/%d，出库单已发货 %d 张，库存流水 %d 条", completedTasks, len(finalTasks), shippedOrders, len(inventoryTrans)), Status: ScenarioStepCompleted},
		{Title: "库存不变量", Detail: invariantMessage, Status: ScenarioStepCompleted},
		{Title: "实验边界", Detail: "页面展示的是模拟 PDA 扫码请求，不表示项目接入或实现了真实 PDA 硬件客户端。", Status: ScenarioStepCompleted},
	}
	return &PickingResult{
		Workers:                        workers,
		Contenders:                     contenders,
		TaskCount:                      len(tasks),
		TotalTarget:                    totalTarget,
		ConcurrentScanAttempts:         concurrentSuccess + competitionRejected,
		ConcurrentSuccess:              concurrentSuccess,
		CompetitionRejected:            competitionRejected,
		StillIncompleteAfterConcurrent: stillIncomplete,
		WorkerSuccess:                  int(workerSuccess.Load()),
		WorkerRejected:                 int(workerRejected.Load()),
		ContenderSuccess:               int(contenderSuccess.Load()),
		ContenderRejected:              int(contenderRejected.Load()),
		CleanupRemainingTasks:          cleanupRemainingTasks,
		CleanupPickedQuantity:          int(cleanupSuccess.Load()),
		CleanupRejected:                int(cleanupRejected.Load()),
		DuplicateScanAttempts:          duplicateAttempts,
		DuplicateScanRejected:          duplicateRejected,
		DuplicateScanSuccess:           duplicateSuccess,
		FinalPicked:                    finalPicked,
		CompletedTasks:                 completedTasks,
		ShippedOrders:                  int(shippedOrders),
		DurationMs:                     durationMs,
		StockTotal:                     stats.StockTotal,
		AvailableTotal:                 stats.AvailableTotal,
		AllocatedTotal:                 stats.AllocatedTotal,
		NegativeRows:                   stats.NegativeRows,
		InvariantOK:                    invariantOK,
		InvariantMessage:               invariantMessage,
		InventoryTrans:                 inventoryTrans,
		Summary:                        summary,
		Steps:                          steps,
	}, nil
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
