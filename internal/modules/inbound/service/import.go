package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/snowflake"
)

func (s *Service) Import(ctx context.Context, fileName string, data []byte) (*dto.ImportResp, error) {
	if err := os.MkdirAll(s.uploadDir, 0o750); err != nil {
		return nil, err
	}
	taskID := fmt.Sprintf("IMP%d", snowflake.Next())
	path := filepath.Join(s.uploadDir, taskID+filepath.Ext(fileName))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	t := &model.ImportTask{
		Base:   sysmodel.Base{ID: snowflake.Next()},
		TaskID: taskID, Status: model.ImportPending,
		FileName: fileName, FilePath: path,
	}
	if err := s.repo.CreateImportTask(ctx, s.tm.DB(), t); err != nil {
		if removeErr := os.Remove(path); removeErr != nil {
			log.WithContext(ctx).Warn("remove orphan import file failed", "err", removeErr)
		}
		return nil, err
	}
	// 任务落库后由 RunImports 领取，HTTP 请求不负责创建工作 goroutine。
	return &dto.ImportResp{TaskID: taskID}, nil
}

func (s *Service) GetImport(ctx context.Context, taskID string) (*model.ImportTask, error) {
	t, err := s.repo.GetImportTask(ctx, s.tm.DB(), taskID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.ImportTaskNotFound
		}
		return nil, err
	}
	return t, nil
}

// ListImports 返回最近 limit 条历史导入任务（下拉筛选器专用，默认 20 条）。
func (s *Service) ListImports(ctx context.Context, limit int) ([]*model.ImportTask, error) {
	if limit <= 0 {
		limit = 20
	}
	return s.repo.ListImportTasks(ctx, s.tm.DB(), limit)
}

// removeImportFile 删除已成功处理的源文件。失败任务保留文件，便于排查
// 格式、解析或业务数据问题；文件路径不会通过 API 暴露给用户。
func (s *Service) removeImportFile(ctx context.Context, task *model.ImportTask) {
	if task == nil || task.FilePath == "" {
		return
	}
	if err := os.Remove(task.FilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.WithContext(ctx).Warn("remove completed import file failed", "task_id", task.TaskID, "err", err)
	}
}
