package service

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

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
		{Title: "PDA 并发模型", Detail: fmt.Sprintf("%d 个拣货员逐件扫码，%d 个抢单者模拟重复扫码/误派", workers, contenders), Status: ScenarioStepCompleted},
		{Title: "测试重点", Detail: "任务行锁、分配行版本、逐件扫码、同一任务抢拣、防超拣、防负库存", Status: ScenarioStepCompleted},
		{Title: "拣货结果", Detail: fmt.Sprintf("正常扫码成功 %d 次，并发/重复扫码拒绝 %d 次", workerSuccess.Load(), workerRejected.Load()+contenderRejected.Load()), Status: ScenarioStepCompleted},
		{Title: "终态校验", Detail: fmt.Sprintf("任务完成 %d/%d，订单已发货 %d 张，最终拣货 %d/%d", completedTasks, len(finalTasks), shippedOrders, finalPicked, totalTarget), Status: ScenarioStepCompleted},
		{Title: "库存快照", Detail: fmt.Sprintf("stock=%d available=%d allocated=%d 负数行=%d", stats.StockTotal, stats.AvailableTotal, stats.AllocatedTotal, stats.NegativeRows), Status: ScenarioStepCompleted},
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
