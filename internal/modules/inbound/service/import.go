package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/snowflake"
)

// Excel 异步导入与悬挂任务补偿。
const (
	importHeartbeatInterval  = 30 * time.Second // 处理中任务心跳间隔
	compensateScanInterval   = 2 * time.Minute  // 悬挂任务扫描间隔
	compensateLockTTL        = 5 * time.Minute  // 补偿分布式锁持有时长
	pendingStaleThreshold    = 2 * time.Minute  // PENDING 超时阈值（超过未被抢占视为悬挂）
	processingStaleThreshold = 5 * time.Minute  // PROCESSING 心跳超时阈值
	staleScanLimit           = 10               // 单次扫描悬挂任务上限
	maxImportErrMsgLen       = 1000             // 导入失败信息落库最大长度
)

const compensateLockKey = "wms:import:compensate"

func excelizeOpenFile(path string) (*excelize.File, error) {
	return excelize.OpenFile(path)
}

func (s *Service) Import(ctx context.Context, fileName string, data []byte) (*dto.ImportResp, error) {
	if err := os.MkdirAll(s.uploadDir, 0o755); err != nil {
		return nil, err
	}
	taskID := fmt.Sprintf("IMP%d", snowflake.Next())
	path := filepath.Join(s.uploadDir, taskID+filepath.Ext(fileName))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, err
	}
	t := &model.ImportTask{
		Base:   sysmodel.Base{ID: snowflake.Next()},
		TaskID: taskID, Status: model.ImportPending,
		FileName: fileName, FilePath: path,
	}
	if err := s.repo.CreateImportTask(ctx, s.tm.DB(), t); err != nil {
		return nil, err
	}
	go s.processImport(taskID) // 异步执行
	return &dto.ImportResp{TaskID: taskID}, nil
}

func (s *Service) GetImport(ctx context.Context, taskID string) (*model.ImportTask, error) {
	t, err := s.repo.GetImportTask(ctx, s.tm.DB(), taskID)
	if err != nil {
		return nil, errcode.ImportTaskNotFound
	}
	return t, nil
}

func (s *Service) processImport(taskID string) {
	ctx := context.Background()
	n, err := s.repo.CASImportStatus(s.tm.DB(), taskID, model.ImportPending, model.ImportProcessing)
	if err != nil || n == 0 { // 已被其他 goroutine/节点抢占
		return
	}
	// 心跳：长任务定期刷新 updated_at，防止被悬挂补偿误判
	stopHeartbeat := make(chan struct{})
	go func() {
		t := time.NewTicker(importHeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				_ = s.repo.TouchImport(s.tm.DB(), taskID)
			case <-stopHeartbeat:
				return
			}
		}
	}()
	defer close(stopHeartbeat)

	t, err := s.repo.GetImportTask(ctx, s.tm.DB(), taskID)
	if err != nil {
		return
	}
	total, success, fail, errMsg := s.doImport(ctx, t)
	status := model.ImportCompleted
	if success == 0 && total > 0 {
		status = model.ImportFailed
	}
	if err := s.repo.FinishImport(s.tm.DB(), taskID, status, total, success, fail, errMsg); err != nil {
		log.L().Error("finish import failed", "task_id", taskID, "err", err)
	}
}

func (s *Service) doImport(ctx context.Context, t *model.ImportTask) (total, success, fail int, errMsg string) {
	f, err := excelizeOpenFile(t.FilePath)
	if err != nil {
		return 0, 0, 1, "打开文件失败: " + err.Error()
	}
	defer f.Close()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
		return 0, 0, 1, errcode.ImportTemplateHeader.Msg
	}
	header := rows[0]
	if len(header) < 3 || header[0] != "仓库编码" || header[1] != "货品编码" || header[2] != "预期数量" {
		return 0, 0, 1, errcode.ImportTemplateHeader.Msg
	}
	var failMsgs []string
	for i, row := range rows[1:] {
		total++
		if len(row) < 3 {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: 列数不足", i+2))
			continue
		}
		var expectedQty int
		if _, err := fmt.Sscanf(strings.TrimSpace(row[2]), "%d", &expectedQty); err != nil || expectedQty <= 0 {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: 预期数量非法", i+2))
			continue
		}
		wh, err := s.basic.GetWarehouseByCode(ctx, strings.TrimSpace(row[0]))
		if err != nil {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: %v", i+2, err))
			continue
		}
		sku, err := s.basic.GetSKUByCode(ctx, strings.TrimSpace(row[1]))
		if err != nil {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: %v", i+2, err))
			continue
		}
		remark := ""
		if len(row) > 3 {
			remark = row[3]
		}
		// 幂等建单：以（导入任务 ID + Excel 行号）为键，补偿重跑不重复建单
		if _, err = s.CreateImportOrder(ctx, t.TaskID, i+2, &dto.CreateOrderReq{
			WarehouseID: wh.ID, Remark: remark,
			Details: []dto.OrderDetailItem{{SKUID: sku.ID, ExpectedQty: expectedQty}},
		}, "import"); err != nil {
			fail++
			failMsgs = append(failMsgs, fmt.Sprintf("第%d行: %v", i+2, err))
			continue
		}
		success++
	}
	if len(failMsgs) > 0 {
		errMsg = strings.Join(failMsgs, "; ")
		if len(errMsg) > maxImportErrMsgLen {
			errMsg = errMsg[:maxImportErrMsgLen]
		}
	}
	return total, success, fail, errMsg
}

func (s *Service) StartCompensator(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(compensateScanInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.compensateOnce()
			}
		}
	}()
}

func (s *Service) compensateOnce() {
	ctx := context.Background()
	// 多实例部署时用分布式锁防止重复补偿；Redis 故障降级为直接执行（单实例语义）。
	if s.locker != nil {
		release, ok, err := s.locker.Lock(ctx, compensateLockKey, compensateLockTTL)
		if err != nil {
			log.L().Warn("compensate lock unavailable, run in standalone mode", "err", err)
		} else if !ok {
			return // 其他实例正在补偿
		} else {
			defer release()
		}
	}
	now := time.Now()
	stale, err := s.repo.ListStaleImports(ctx, s.tm.DB(),
		now.Add(-pendingStaleThreshold), now.Add(-processingStaleThreshold), staleScanLimit)
	if err != nil {
		log.L().Error("scan stale imports failed", "err", err)
		return
	}
	for _, t := range stale {
		if t.Status == model.ImportProcessing { // 心跳超时的 PROCESSING：复位后重跑
			n, err := s.repo.ResetProcessingToPending(s.tm.DB(), t.TaskID)
			if err != nil || n == 0 {
				continue
			}
			log.L().Warn("stale processing import reset", "task_id", t.TaskID)
		}
		// PENDING：CAS 抢占后重跑
		go s.processImport(t.TaskID)
	}
}
