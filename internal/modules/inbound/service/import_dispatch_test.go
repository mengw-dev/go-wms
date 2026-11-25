package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/config"
)

// Pause just after an empty queue read. A notification received before the
// worker enters its wait must still trigger another read.
func TestImportWakeDuringEmptyScan(t *testing.T) {
	s, db, ctx := importFixture(t)
	data := importWorkbook(t, [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", 2}})
	scanned := make(chan struct{})
	resume := make(chan struct{})
	var firstScan sync.Once
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := db.Callback().Query().After("gorm:query").Register("test:empty_import_scan", func(q *gorm.DB) {
		if q.Statement.Context != workerCtx || q.Statement.Table != "wms_import_task" || q.RowsAffected != 0 || q.Error != nil {
			return
		}
		firstScan.Do(func() {
			close(scanned)
			select {
			case <-resume:
			case <-workerCtx.Done():
			}
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	startImportWorker(workerCtx, t, cancel, s, importFallbackInterval)
	select {
	case <-scanned:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not scan the empty queue")
	}

	// The worker cannot receive yet. Multiple uploads must finish with at most
	// one buffered wake, and all files must still be processed.
	type uploads struct {
		ids []string
		err error
	}
	enqueued := make(chan uploads, 1)
	go func() {
		var result uploads
		for range 3 {
			resp, err := s.Import(ctx, "orders.xlsx", data)
			if err != nil {
				result.err = err
				break
			}
			result.ids = append(result.ids, resp.TaskID)
		}
		enqueued <- result
	}()
	var ids []string
	select {
	case result := <-enqueued:
		if result.err != nil {
			t.Fatal(result.err)
		}
		ids = result.ids
	case <-time.After(5 * time.Second):
		t.Fatal("uploads blocked while the worker was unable to receive")
	}
	close(resume)
	for _, id := range ids {
		waitImportComplete(ctx, t, s, id)
	}
	var orders int64
	if err := db.WithContext(ctx).Model(&model.ReceiptOrder{}).Count(&orders).Error; err != nil || orders != 3 {
		t.Fatalf("expected one order per upload, got %d: %v", orders, err)
	}
}

func TestImportFallbackFindsTaskFromAnotherInstance(t *testing.T) {
	s, db, ctx := importFixture(t)
	other := New(s.repo, s.tm, s.no, s.basic, s.inv, s.taskAPI, config.UploadConfig{Dir: s.uploadDir}, s.limits)
	data := importWorkbook(t, [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", 2}})
	workerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	scanned := make(chan struct{})
	var firstScan sync.Once
	err := db.Callback().Query().After("gorm:query").Register("test:import_fallback_scan", func(q *gorm.DB) {
		if q.Statement.Context == workerCtx && q.Statement.Table == "wms_import_task" && q.RowsAffected == 0 && q.Error == nil {
			firstScan.Do(func() { close(scanned) })
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	startImportWorker(workerCtx, t, cancel, s, 50*time.Millisecond)
	select {
	case <-scanned:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not scan the empty queue")
	}
	resp, err := other.Import(ctx, "orders.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	waitImportComplete(ctx, t, s, resp.TaskID)
}

func startImportWorker(ctx context.Context, t *testing.T, cancel context.CancelFunc, s *Service, fallback time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runImports(ctx, fallback)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("worker did not stop")
		}
	})
}

func waitImportComplete(ctx context.Context, t *testing.T, s *Service, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		task, err := s.GetImport(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		switch task.Status {
		case model.ImportCompleted:
			if task.SuccessRows != 1 || task.FailRows != 0 {
				t.Fatalf("unexpected import result: %+v", task)
			}
			return
		case model.ImportFailed:
			t.Fatalf("import failed: %s", task.ErrorMsg)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("task %s did not complete after wake or fallback", id)
		}
	}
}
