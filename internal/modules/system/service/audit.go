package service

import (
	"context"
	"log/slog"
	"time"

	"gowms/internal/modules/system/dto"
	"gowms/internal/modules/system/model"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/snowflake"
)

// 操作日志异步写入和查询。

func (s *Service) Record(_ context.Context, r middleware.OperLogRecord) {
	item := &model.SysOperLog{
		ID: snowflake.Next(), UserID: r.UserID, Username: r.Username,
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

func (s *Service) consumeOperLogs() {
	batch := make([]*model.SysOperLog, 0, 100)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case item := <-s.logCh:
			batch = append(batch, item)
			if len(batch) >= 100 {
				s.flushOperLogs(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flushOperLogs(batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Service) flushOperLogs(batch []*model.SysOperLog) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.repo.InsertOperLogs(ctx, batch); err != nil {
		slog.Error("insert oper logs failed", "err", err, "count", len(batch))
	}
}

func (s *Service) ListOperLogs(ctx context.Context, q *dto.OperLogQuery) ([]*model.SysOperLog, int64, error) {
	return s.repo.ListOperLogs(ctx, q.Username, q.Path, q.Page, q.PageSize)
}
