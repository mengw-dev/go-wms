package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gowms/internal/pkg/log"
)

// 导入文件生命周期：
// 成功任务在完成时立即删除源文件（见 import_worker.go）；失败任务保留一段时间用于排查，
// 到期由 CleanupExpiredImportFiles 删除；上传目录中没有对应任务记录的孤儿文件
// （落库失败残留、任务被删除或演示重置后的遗留）一并清理。

const (
	// defaultFailedFileRetention 直接构造 Service 未配置保留期时的兜底值（正常路径由 config 提供默认 72h）。
	defaultFailedFileRetention = 72 * time.Hour
	importCleanupInterval      = 1 * time.Hour
	importCleanupBatch         = 200
	// importFilePrefix 上传文件名前缀（IMP<雪花ID>.xlsx）；孤儿清理只处理该前缀，避免误删目录内其他文件。
	importFilePrefix = "IMP"
	// importOrphanGrace 孤儿文件的最小存活时间：上传写文件与任务落库之间存在毫秒级窗口，
	// 刚写入的文件不能在下一次清理时被当成孤儿删除。
	importOrphanGrace = importCleanupInterval
)

// CleanupExpiredImportFiles 执行一轮导入文件清理：
//  1. 删除超过保留期仍保留源文件的失败任务文件，并置空路径；
//  2. 删除上传目录中没有对应任务记录、且已超过保护期的孤儿文件。
func (s *Service) CleanupExpiredImportFiles(ctx context.Context) error {
	before := time.Now().Add(-s.failedFileRetention)
	expired, err := s.repo.ListExpiredFailedImports(ctx, s.tm.DB(), before, importCleanupBatch)
	if err != nil {
		return err
	}
	for _, task := range expired {
		if err := os.Remove(task.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.WithContext(ctx).Warn("remove expired import file failed", "task_id", task.TaskID, "err", err)
			continue // 文件仍在：保留路径，下一轮重试
		}
		if err := s.repo.ClearImportFilePath(ctx, s.tm.DB(), task.TaskID, task.FilePath); err != nil {
			return err
		}
	}
	return s.removeOrphanImportFiles(ctx)
}

// removeOrphanImportFiles 删除上传目录中不存在对应任务记录的导入文件。
func (s *Service) removeOrphanImportFiles(ctx context.Context) error {
	entries, err := os.ReadDir(s.uploadDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // 尚未产生任何上传
		}
		return err
	}
	paths, err := s.repo.ListImportFilePaths(ctx, s.tm.DB())
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		known[filepath.Base(p)] = struct{}{}
	}
	graceBefore := time.Now().Add(-importOrphanGrace)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, importFilePrefix) {
			continue
		}
		if _, ok := known[name]; ok {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(graceBefore) {
			continue // 刚写入的文件可能还在落库过程中
		}
		if err := os.Remove(filepath.Join(s.uploadDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.WithContext(ctx).Warn("remove orphan import file failed", "file", name, "err", err)
		}
	}
	return nil
}

// RunImportFileCleanup 周期清理导入文件；调用方取消并等待本方法退出后才能关闭数据库。
func (s *Service) RunImportFileCleanup(ctx context.Context) {
	ticker := time.NewTicker(importCleanupInterval)
	defer ticker.Stop()
	for {
		if err := s.CleanupExpiredImportFiles(ctx); err != nil && ctx.Err() == nil {
			log.L().Error("cleanup import files failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
