package xlsxexport

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"gowms/internal/pkg/errcode"
)

func TestWorkbookTextNumbersEmptyAndMetadata(t *testing.T) {
	payload := `=HYPERLINK("https://invalid.example","do not execute")`
	file, err := Build(context.Background(), "test", "库存", []string{"编码", "ID", "名称", "数量", "实盘"}, [][]any{{"000123", "9223372036854775800", payload, 0, nil}, {"+cmd", "-001", "中文\n名称", -4, 0}}, `{"in_stock_only":false}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(file.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	for cell, want := range map[string]string{"A2": "000123", "B2": "9223372036854775800", "C2": payload, "D2": "0", "E2": "", "D3": "-4", "E3": "0"} {
		got, err := book.GetCellValue("数据", cell)
		if err != nil || got != want {
			t.Fatalf("%s=%q want %q err=%v", cell, got, want, err)
		}
		formula, err := book.GetCellFormula("数据", cell)
		if err != nil || formula != "" {
			t.Fatalf("formula in %s: %q %v", cell, formula, err)
		}
	}
	kind, _ := book.GetCellType("数据", "B2")
	if kind != excelize.CellTypeSharedString {
		t.Fatalf("ID stored as %v", kind)
	}
	kind, _ = book.GetCellType("数据", "D2")
	if kind == excelize.CellTypeSharedString {
		t.Fatal("quantity is text")
	}
	rows, err := book.GetRows("说明")
	if err != nil || rows[2][1] != "2" || rows[3][1] != `{"in_stock_only":false}` {
		t.Fatalf("metadata=%v err=%v", rows, err)
	}
	empty, err := Build(context.Background(), "empty", "空结果", []string{"列"}, nil, "{}", nil)
	if err != nil {
		t.Fatal(err)
	}
	e, err := excelize.OpenReader(bytes.NewReader(empty.Data))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	data, err := e.GetRows("数据")
	if err != nil || len(data) != 1 {
		t.Fatalf("empty rows=%v err=%v", data, err)
	}
	if Time(time.Date(2026, 10, 22, 0, 0, 0, 0, time.UTC)) != "2026-10-22 08:00:00" {
		t.Fatal("timezone")
	}
}

func TestExportLimitsCancellationAndSlots(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := Build(ctx, "a", "a", []string{"x"}, nil, "", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	_, release1, err := Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release1()
	bounded, release2, err := Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release2()
	if deadline, ok := bounded.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
		t.Fatal("missing export deadline")
	}
	if _, _, err := Start(context.Background()); errcode.From(err).HTTPStatus != 429 {
		t.Fatalf("not bounded: %v", err)
	}
	if _, err := Build(context.Background(), "a", "a", []string{"x"}, make([][]any, MaxRows+1), "", nil); !errors.Is(err, TooManyRows) {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), "a", "a", []string{"x"}, [][]any{{strings.Repeat("x", 32768)}}, "", nil); err == nil {
		t.Fatal("silently truncated text")
	}
	rows := make([][]any, MaxRows)
	for i := range rows {
		rows[i] = []any{i}
	}
	if _, err := Build(context.Background(), "a", "a", []string{"x"}, rows, "", nil); err != nil {
		t.Fatalf("boundary rejected: %v", err)
	}
}
