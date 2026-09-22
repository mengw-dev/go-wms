package service

import (
	"context"
	"log/slog"
	"time"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

// 操作日志异步写入和查询。

func (s *Service) Record(ctx context.Context, r middleware.OperLogRecord) {
	item := &model.SysOperLog{
		TenantID: tenant.FromContext(ctx),
		ID:       snowflake.Next(), UserID: r.UserID, Username: r.Username,
		Path: r.Path, Method: r.Method, Params: r.Params, IP: r.IP,
		CostMs: r.CostMs, Status: r.Status, Result: r.Result,
		CreatedAt: time.Now(),
	}
	select {
	case s.logCh <- item:
	default:
		slog.Warn("oper log channel full, dropped", "path", r.Path)
	}
}

// RunOperLogs 消费操作日志，阻塞至 ctx 取消并尝试写完已入队的日志。
// 调用方只启动一个消费者，并在 HTTP 请求结束后取消、等待它退出，再关闭数据库。
// 队列满或写库失败仍可能丢日志；库存流水必须由业务事务单独保证。
func (s *Service) RunOperLogs(ctx context.Context) {
	batch := make([]*model.SysOperLog, 0, 100)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// 停止期间共用一个超时，避免每个批次分别等待 5 秒。
			flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			for remaining := len(s.logCh); remaining > 0; remaining-- {
				batch = append(batch, <-s.logCh)
				if len(batch) >= 100 {
					s.flushOperLogs(flushCtx, batch)
					batch = batch[:0]
				}
			}
			if len(batch) > 0 {
				s.flushOperLogs(flushCtx, batch)
			}
			return
		case item := <-s.logCh:
			batch = append(batch, item)
			if len(batch) >= 100 {
				s.flushOperLogs(context.Background(), batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushOperLogs(context.Background(), batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Service) flushOperLogs(ctx context.Context, batch []*model.SysOperLog) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.repo.InsertOperLogs(ctx, batch); err != nil {
		slog.Error("insert oper logs failed", "err", err, "count", len(batch))
	}
}

func (s *Service) ListOperLogs(ctx context.Context, q *dto.OperLogQuery) ([]*model.SysOperLog, int64, error) {
	return s.repo.ListOperLogs(ctx, q.Username, q.Path, q.Page, q.PageSize)
}
