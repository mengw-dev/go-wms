package service

import (
	"context"
	"fmt"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/log"
	"gowms/internal/pkg/quota"
)

const maxImportMessageRunes = 1000

type importResult struct {
	Total       int
	Success     int
	Failed      int
	Message     string
	Interrupted bool
}

func (r importResult) status() model.ImportTaskStatus {
	if r.Interrupted || r.Success == 0 && (r.Failed > 0 || r.Message != "") {
		return model.ImportFailed
	}
	return model.ImportCompleted
}

func (s *Service) doImport(ctx context.Context, task *model.ImportTask) (result importResult) {
	// 文件是外部输入，解析器 panic 转为明确的任务失败，保留已处理的行数。
	defer func() {
		if r := recover(); r != nil {
			log.L().Error("import panic recovered", "task_id", task.TaskID, "panic", r, "stack", string(debug.Stack()))
			result.Message = "导入中断，请确认文件未损坏后重试"
			result.Interrupted = true
		}
	}()
	// xlsx 是 ZIP 文件，请求体大小不能限制解压后的内存占用。
	f, err := excelize.OpenFile(task.FilePath, excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 8 << 20})
	if err != nil {
		log.L().Warn("open import file failed", "task_id", task.TaskID, "err", err)
		return importResult{Message: "打开文件失败，请确认文件存在且格式正确"}
	}
	defer func() { _ = f.Close() }()
	rows, err := f.GetRows(f.GetSheetName(0))
	if err != nil || len(rows) < 2 {
		return importResult{Message: errcode.ImportTemplateHeader.Msg}
	}
	header := rows[0]
	if len(header) < 3 || header[0] != "仓库编码" || header[1] != "货品编码" || header[2] != "预期数量" {
		return importResult{Message: errcode.ImportTemplateHeader.Msg}
	}
	if err := quota.GuardImportRows(ctx, len(rows)-1, s.limits.MaxImportRows); err != nil {
		return importResult{Total: len(rows) - 1, Failed: len(rows) - 1, Message: errcode.From(err).Msg}
	}
	for i, row := range rows[1:] {
		if ctx.Err() != nil {
			return result
		}
		result.Total++
		if err := s.importRow(ctx, task, i+2, row); err != nil {
			result.Failed++
			businessErr := errcode.From(err)
			if businessErr.Code == errcode.Internal.Code {
				log.L().Error("import row failed", "task_id", task.TaskID, "row", i+2, "err", err)
			}
			// 保留有限的错误摘要，按字符截断，避免截断中文 UTF-8 字节。
			message := result.Message + fmt.Sprintf("第%d行: %s; ", i+2, businessErr.Msg)
			runes := []rune(message)
			result.Message = string(runes[:min(len(runes), maxImportMessageRunes)])
			continue
		}
		result.Success++
	}
	return result
}

func (s *Service) importRow(ctx context.Context, task *model.ImportTask, rowNo int, row []string) error {
	if len(row) < 3 {
		return errcode.New(errcode.ParamError.Code, "列数不足")
	}
	quantity, err := strconv.Atoi(strings.TrimSpace(row[2]))
	if err != nil || quantity <= 0 {
		return errcode.New(errcode.ParamError.Code, "预期数量必须为正整数")
	}
	warehouse, err := s.basic.GetWarehouseByCode(ctx, strings.TrimSpace(row[0]))
	if err != nil {
		return err
	}
	sku, err := s.basic.GetSKUByCode(ctx, strings.TrimSpace(row[1]))
	if err != nil {
		return err
	}
	var remark string
	if len(row) > 3 {
		remark = row[3]
	}
	_, err = s.createImportOrder(ctx, task, rowNo, &dto.CreateOrderReq{
		WarehouseID: warehouse.ID, Remark: remark,
		Details: []dto.OrderDetailItem{{SKUID: sku.ID, ExpectedQty: quantity}},
	}, "import")
	return err
}
