package service

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/errcode"
)

// 小体积的工作簿也能声明超大行号；必须在业务写入前拒绝，不能按行号展开导入。
func TestImportRejectsOutOfRangeWorksheetRow(t *testing.T) {
	data := importWorkbook(t, [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", 1}})
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	modified := false
	for _, entry := range reader.File {
		file, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			t.Fatalf("read workbook entry: %v %v", readErr, closeErr)
		}
		if entry.Name == "xl/worksheets/sheet1.xml" {
			invalidRow := fmt.Appendf(nil, `<row r="%d"`, excelize.TotalRows+1)
			replaced := bytes.Replace(content, []byte(`<row r="2"`), invalidRow, 1)
			modified = !bytes.Equal(content, replaced)
			content = replaced
		}
		dst, createErr := writer.Create(entry.Name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := dst.Write(content); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if !modified {
		t.Fatal("fixture did not replace the worksheet row number")
	}
	path := filepath.Join(t.TempDir(), "out-of-range.xlsx")
	if err := os.WriteFile(path, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	// 无业务依赖：有效拒绝必须发生在任何 importRow/数据库访问之前。
	result := (&Service{}).doImport(context.Background(), &model.ImportTask{FilePath: path})
	if result.status() != model.ImportFailed || result.Interrupted || result.Total != 0 || result.Success != 0 || result.Failed != 0 || result.Message != errcode.ImportTemplateHeader.Msg {
		t.Fatalf("out-of-range row reached business processing: %+v", result)
	}
}
