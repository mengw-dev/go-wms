package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/tenant"
)

const (
	importFallbackInterval   = 30 * time.Second
	importErrorRetryInterval = 2 * time.Second
	importHeartbeatInterval  = 30 * time.Second
	importTimeout            = 10 * time.Minute
	compensateScanInterval   = 2 * time.Minute
	processingStaleThreshold = 5 * time.Minute
	staleScanLimit           = 10
)

func (s *Service) wakeImports() {
	select {
	case s.importWake <- struct{}{}:
	default:
	}
}

// RunImports 启动时先检查积压，查空后等待本地唤醒或兜底扫描。
// 每个实例串行处理文件，多实例由数据库条件更新竞争任务。
func (s *Service) RunImports(ctx context.Context) {
	s.runImports(ctx, importFallbackInterval)
}

func (s *Service) runImports(ctx context.Context, fallbackInterval time.Duration) {
	for ctx.Err() == nil {
		// 读库前清掉已有提示。扫描期间到达的新提示仍可唤醒下一轮，
		// 包括 SELECT 已查空、worker 尚未进入等待的间隙。
		select {
		case <-s.importWake:
		default:
		}
		task, err := s.repo.NextPendingImport(ctx, s.tm.DB())
		delay, wake := fallbackInterval, s.importWake
		if err == nil {
			err = s.processImport(ctx, task)
			if err == nil {
				continue
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		if err != nil {
			if ctx.Err() == nil {
				log.L().Error("import worker failed", "err", err)
			}
			// 故障期间按原间隔重试，上传通知不会绕过等待。
			delay, wake = importErrorRetryInterval, nil
		}
		select {
		case <-ctx.Done():
			return
		case <-wake:
		case <-time.After(delay):
		}
	}
}

func (s *Service) processImport(parent context.Context, task *model.ImportTask) error {
	ctx, cancel := context.WithTimeout(tenant.WithExactTenant(parent, task.TenantID), importTimeout)
	defer cancel()
	token := uuid.NewString()
	claimed, err := s.repo.ClaimImport(s.tm.DB().WithContext(ctx), task, token)
	if err != nil || !claimed {
		return err
	}
	task.RunToken = token
	start := time.Now()
	outcome := "error"
	defer func() { s.metrics.RecordImport(outcome, time.Since(start)) }()

	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		s.heartbeatImport(ctx, cancel, task)
	}()
	defer func() { cancel(); <-heartbeatDone }()
	result := s.doImport(ctx, task)

	// 请求/进程取消后仍需有限时地保存状态；所有写入都校验本次 token。
	finishCtx, finishCancel := context.WithTimeout(tenant.WithExactTenant(context.Background(), task.TenantID), 5*time.Second)
	defer finishCancel()
	db := s.tm.DB().WithContext(finishCtx)
	if parent.Err() != nil || errors.Is(ctx.Err(), context.Canceled) {
		released, err := s.repo.ReleaseImport(db, task)
		if err == nil {
			outcome = "lost"
			if released {
				outcome = "released"
			}
		}
		return err
	}
	var status model.ImportTaskStatus
	message := result.Message
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		status = model.ImportFailed
		message = "导入超时，请检查文件后重新上传"
	} else {
		status = result.status()
	}
	finished, err := s.repo.FinishImport(db, task, status, result.Total, result.Success, result.Failed, message)
	if err != nil || !finished {
		if err == nil {
			outcome = "lost"
		}
		return err
	}
	outcome = "failed"
	if status == model.ImportCompleted {
		outcome = "completed"
		if result.Failed > 0 {
			outcome = "partial"
		}
		s.removeImportFile(finishCtx, task)
	}
	return nil
}

func (s *Service) heartbeatImport(ctx context.Context, cancel context.CancelFunc, task *model.ImportTask) {
	ticker := time.NewTicker(importHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beatCtx, stop := context.WithTimeout(ctx, 5*time.Second)
			owned, err := s.repo.TouchImport(s.tm.DB().WithContext(beatCtx), task)
			stop()
			if err != nil {
				log.L().Warn("import heartbeat failed", "task_id", task.TaskID, "err", err)
			} else if !owned {
				cancel()
				return
			}
		}
	}
}

// RunCompensator 只归还超时任务，不启动新 worker。数据库条件更新可安全处理并发扫描。
func (s *Service) RunCompensator(ctx context.Context) {
	ticker := time.NewTicker(compensateScanInterval)
	defer ticker.Stop()
	for {
		s.compensateOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) compensateOnce(ctx context.Context) {
	before := time.Now().Add(-processingStaleThreshold)
	tasks, err := s.repo.ListStaleImports(ctx, s.tm.DB(), before, staleScanLimit)
	if err != nil {
		if ctx.Err() == nil {
			log.L().Error("scan stale imports failed", "err", err)
		}
		return
	}
	for _, task := range tasks {
		reset, err := s.repo.ResetStaleImport(s.tm.DB().WithContext(ctx), task, before)
		if err != nil {
			log.L().Error("reset stale import failed", "task_id", task.TaskID, "err", err)
			continue
		}
		if reset {
			s.wakeImports()
			s.metrics.RecordImportRecovery()
			log.L().Warn("stale import returned to pending", "task_id", task.TaskID)
		}
	}
}
