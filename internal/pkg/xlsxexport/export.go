// Package xlsxexport builds bounded, read-only Excel downloads shared by business modules.
package xlsxexport

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"

	"gowms/internal/pkg/errcode"
)

const MaxRows = 10000

var slots = make(chan struct{}, 2)
var TooManyRows = errcode.New(400, "导出超过10000行，请缩小筛选范围后重试")
var exportZone = time.FixedZone("UTC+08:00", 8*60*60)

type File struct {
	Name string
	Data []byte
}

// Start bounds concurrent exports per process and gives reads/building a shared budget.
// The caller must defer release, including on query or workbook errors.
func Start(ctx context.Context) (context.Context, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	select {
	case slots <- struct{}{}:
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		return bounded, func() { cancel(); <-slots }, nil
	default:
		return nil, nil, errcode.NewHTTP(429, "导出任务较多，请稍后重试", 429)
	}
}

// Time renders all export timestamps with an explicit, fixed UTC+08:00 convention.
func Time(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(exportZone).Format("2006-01-02 15:04:05")
}

// Build stores every string as a string cell, never as a formula or numeric ID.
// There is no disk artifact or long database transaction during XLSX serialization.
func Build(ctx context.Context, prefix, title string, columns []string, rows [][]any, filters string, notes []string) (file *File, err error) {
	if len(rows) > MaxRows {
		return nil, TooManyRows
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	defer func() { err = errors.Join(err, book.Close()) }()
	if err = book.SetSheetName("Sheet1", "数据"); err != nil {
		return nil, err
	}
	if _, err = book.NewSheet("说明"); err != nil {
		return nil, err
	}
	header := make([]any, len(columns))
	for i, column := range columns {
		header[i] = column
	}
	if err = writeRow(book, "数据", 1, header); err != nil {
		return nil, err
	}
	for i, row := range rows {
		if i%128 == 0 {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
		}
		if len(row) != len(columns) {
			return nil, fmt.Errorf("export row %d has invalid column count", i)
		}
		if err = writeRow(book, "数据", i+2, row); err != nil {
			return nil, err
		}
	}
	lastColumn, err := excelize.ColumnNumberToName(len(columns))
	if err != nil {
		return nil, err
	}
	style, err := book.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"334155"}, Pattern: 1}})
	if err != nil {
		return nil, err
	}
	if err = book.SetCellStyle("数据", "A1", lastColumn+"1", style); err != nil {
		return nil, err
	}
	if err = book.SetColWidth("数据", "A", lastColumn, 22); err != nil {
		return nil, err
	}
	if err = book.SetPanes("数据", &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		if err = book.AutoFilter("数据", "A1:"+lastColumn+strconv.Itoa(len(rows)+1), nil); err != nil {
			return nil, err
		}
	}
	info := [][]any{{"导出内容", title}, {"读取完成时间（UTC+08:00）", Time(time.Now())}, {"数据行数", len(rows)}, {"筛选条件", filters}, {"导出范围", "全部匹配记录，忽略列表页码；最多10000行，超限拒绝，不截断"}, {"数据口径", "单次SQL读取的当前数据；资料名称为当前可见值，不代表任意历史时点快照"}, {"文本与数量", "编码、条码、ID按文本保存；数量按各货品库存单位，不混合汇总"}}
	for _, note := range notes {
		info = append(info, []any{"说明", note})
	}
	for i, row := range info {
		if err = writeRow(book, "说明", i+1, row); err != nil {
			return nil, err
		}
	}
	if err = book.SetColWidth("说明", "A", "A", 30); err != nil {
		return nil, err
	}
	if err = book.SetColWidth("说明", "B", "B", 110); err != nil {
		return nil, err
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &File{Name: prefix + "-" + time.Now().In(exportZone).Format("20060102-150405") + ".xlsx", Data: buffer.Bytes()}, nil
}

func writeRow(book *excelize.File, sheet string, row int, values []any) error {
	for i, value := range values {
		cell, err := excelize.CoordinatesToCellName(i+1, row)
		if err != nil {
			return err
		}
		if text, ok := value.(string); ok {
			if utf8.RuneCountInString(text) > 32767 {
				return errcode.New(400, "存在超过Excel单元格长度的文本，无法完整导出")
			}
			if err = book.SetCellStr(sheet, cell, text); err != nil {
				return err
			}
		} else if value != nil {
			if err = book.SetCellValue(sheet, cell, value); err != nil {
				return err
			}
		}
	}
	return nil
}
