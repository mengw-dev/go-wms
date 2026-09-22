package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	"gowms/internal/modules/inbound/dto"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func importFixture(t *testing.T) (*Service, *gorm.DB, context.Context) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &basicmodel.Warehouse{}, &basicmodel.SKU{}, &model.ImportTask{}, &model.ReceiptOrder{}, &model.ReceiptOrderDetail{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithTenant(context.Background(), 11)
	for _, row := range []any{
		&basicmodel.Warehouse{Base: sysmodel.Base{ID: 1}, Code: "WH", Name: "Warehouse", Status: 1},
		&basicmodel.SKU{Base: sysmodel.Base{ID: 1}, Code: "SKU", Barcode: "BAR", Name: "Item", Status: 1},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	tm := tx.New(db)
	basic := basicservice.New(basicrepo.New(), tm, nil, nil, config.LimitsConfig{})
	return New(repository.New(), tm, orderno.New(nil), basic, nil, nil, t.TempDir(), config.LimitsConfig{}), db, ctx
}

func importWorkbook(t *testing.T, rows [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestImportFileFailuresAndStrictQuantity(t *testing.T) {
	for _, tt := range []struct {
		name            string
		rows            [][]any
		success, failed int
		status          model.ImportTaskStatus
	}{
		{"corrupt file", nil, 0, 0, model.ImportFailed},
		{"empty sheet", [][]any{{"仓库编码", "货品编码", "预期数量"}}, 0, 0, model.ImportFailed},
		{"partial success", [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", "2"}, {"WH", "SKU", "12oops"}, {"WH", "SKU", "1.5"}}, 1, 2, model.ImportCompleted},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, db, ctx := importFixture(t)
			data := []byte("invalid workbook")
			if tt.rows != nil {
				data = importWorkbook(t, tt.rows)
			}
			resp, err := s.Import(ctx, "input.xlsx", data)
			if err != nil {
				t.Fatal(err)
			}
			task, err := s.GetImport(ctx, resp.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Status != model.ImportPending {
				t.Fatal("HTTP upload started an unmanaged worker")
			}
			if err := s.processImport(context.Background(), task); err != nil {
				t.Fatal(err)
			}
			got, err := s.GetImport(ctx, task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tt.status || got.SuccessRows != tt.success || got.FailRows != tt.failed {
				t.Fatalf("task=%+v", got)
			}
			_, statErr := os.Stat(task.FilePath)
			if tt.status == model.ImportCompleted && !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("completed import file should be removed: %v", statErr)
			}
			if tt.status == model.ImportFailed && statErr != nil {
				t.Fatalf("failed import file should be retained: %v", statErr)
			}
			if !utf8.ValidString(got.ErrorMsg) {
				t.Fatal("invalid UTF-8 error message")
			}
			var orders int64
			if err := db.Model(&model.ReceiptOrder{}).Count(&orders).Error; err != nil {
				t.Fatal(err)
			}
			if orders != int64(tt.success) {
				t.Fatalf("created orders=%d", orders)
			}
		})
	}
}

func TestImportOwnershipAndIdempotency(t *testing.T) {
	s, db, ctx := importFixture(t)
	task := &model.ImportTask{TaskID: "test-task", Status: model.ImportPending, FilePath: filepath.Join(t.TempDir(), "missing.xlsx")}
	if err := db.WithContext(ctx).Create(task).Error; err != nil {
		t.Fatal(err)
	}
	ownedDB := db.WithContext(ctx)
	if ok, err := s.repo.ClaimImport(ownedDB, task, "old"); err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	task.RunToken = "old"
	if ok, err := s.repo.ClaimImport(ownedDB, task, "second"); err != nil || ok {
		t.Fatalf("duplicate claim: %v %v", ok, err)
	}
	old := *task
	if released, err := s.repo.ReleaseImport(ownedDB, task); err != nil || !released {
		t.Fatalf("release import: released=%v err=%v", released, err)
	}
	if ok, err := s.repo.ClaimImport(ownedDB, task, "new"); err != nil || !ok {
		t.Fatalf("reclaim: %v %v", ok, err)
	}
	task.RunToken = "new"
	for range 3 {
		if ok, err := s.repo.TouchImport(ownedDB, task); err != nil || !ok {
			t.Fatalf("rapid heartbeat: %v %v", ok, err)
		}
	}
	if ok, err := s.repo.TouchImport(ownedDB, &old); err != nil || ok {
		t.Fatalf("old heartbeat: %v %v", ok, err)
	}
	req := &dto.CreateOrderReq{WarehouseID: 1, Details: []dto.OrderDetailItem{{SKUID: 1, ExpectedQty: 2}}}
	if _, err := s.createImportOrder(ctx, &old, 2, req, "test"); err == nil {
		t.Fatal("old execution created an order")
	}
	first, err := s.createImportOrder(ctx, task, 2, req, "test")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.createImportOrder(ctx, task, 2, req, "test")
	if err != nil || again.ID != first.ID {
		t.Fatalf("idempotency: %+v %v", again, err)
	}
	if finished, err := s.repo.FinishImport(ownedDB, &old, model.ImportFailed, 0, 0, 1, "old failure"); err != nil || finished {
		t.Fatalf("old worker finish: finished=%v err=%v", finished, err)
	}
	got, err := s.GetImport(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.ImportProcessing || got.RunToken != "new" {
		t.Fatalf("old worker overwrote new owner: %+v", got)
	}
	before := time.Now().Add(-processingStaleThreshold)
	if err := db.Model(task).Update("updated_at", before.Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	stale, err := s.repo.ListStaleImports(context.Background(), db, before, 10)
	if err != nil || len(stale) != 1 {
		t.Fatalf("stale=%v err=%v", stale, err)
	}
	if ok, err := s.repo.TouchImport(ownedDB, task); err != nil || !ok {
		t.Fatalf("heartbeat: %v %v", ok, err)
	}
	if ok, err := s.repo.ResetStaleImport(ownedDB, stale[0], before); err != nil || ok {
		t.Fatalf("fresh heartbeat was reset: %v %v", ok, err)
	}
}

func TestInterruptedImportIsFailedEvenAfterPartialSuccess(t *testing.T) {
	result := importResult{Total: 2, Success: 1, Interrupted: true, Message: "interrupted"}
	if result.status() != model.ImportFailed {
		t.Fatal("interrupted execution was marked completed")
	}
}

func TestImportWorkerResumesPendingAndStops(t *testing.T) {
	s, _, ctx := importFixture(t)
	resp, err := s.Import(ctx, "bad.xlsx", []byte("bad"))
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); s.RunImports(workerCtx) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline.C:
			t.Fatal("pending task was not processed")
		case <-ticker.C:
			task, err := s.GetImport(ctx, resp.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			if task.Status == model.ImportFailed {
				cancel()
				select {
				case <-done:
					return
				case <-time.After(time.Second):
					t.Fatal("worker did not stop")
				}
			}
		}
	}
}
