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
	importPollInterval       = 2 * time.Second
	importHeartbeatInterval  = 30 * time.Second
	importTimeout            = 10 * time.Minute
	compensateScanInterval   = 2 * time.Minute
	processingStaleThreshold = 5 * time.Minute
	staleScanLimit           = 10
)

// RunImports 每个实例运行一个 worker，串行处理文件；多实例用数据库条件更新竞争任务。
// 请求仅落库，服务重启后仍可领取 PENDING 任务。调用方取消并等待本方法退出后才能关闭数据库。
func (s *Service) RunImports(ctx context.Context) {
	for ctx.Err() == nil {
		task, err := s.repo.NextPendingImport(ctx, s.tm.DB())
		if err == nil {
			err = s.processImport(ctx, task)
			if err == nil {
				continue
			}
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) && ctx.Err() == nil {
			log.L().Error("import worker failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(importPollInterval):
		}
	}
}

func (s *Service) processImport(parent context.Context, task *model.ImportTask) error {
	ctx, cancel := context.WithTimeout(tenant.WithTenant(parent, task.TenantID), importTimeout)
	defer cancel()
	token := uuid.NewString()
	claimed, err := s.repo.ClaimImport(s.tm.DB().WithContext(ctx), task, token)
	if err != nil || !claimed {
		return err
	}
	task.RunToken = token

	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		s.heartbeatImport(ctx, cancel, task)
	}()
	defer func() { cancel(); <-heartbeatDone }()
	result := s.doImport(ctx, task)

	// 请求/进程取消后仍需有限时地保存状态；所有写入都校验本次 token。
	finishCtx, finishCancel := context.WithTimeout(tenant.WithTenant(context.Background(), task.TenantID), 5*time.Second)
	defer finishCancel()
	db := s.tm.DB().WithContext(finishCtx)
	if parent.Err() != nil || errors.Is(ctx.Err(), context.Canceled) {
		_, err := s.repo.ReleaseImport(db, task)
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
		return err
	}
	if status == model.ImportCompleted {
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
			log.L().Warn("stale import returned to pending", "task_id", task.TaskID)
		}
	}
}
