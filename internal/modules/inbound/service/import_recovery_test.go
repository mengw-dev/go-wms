package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	"gowms/internal/modules/inbound/model"
	"gowms/internal/modules/inbound/repository"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/observability"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
)

func TestImportMissingSourceTerminatesWithoutOrders(t *testing.T) {
	s, db, ctx := importFixture(t)
	m := observability.New(nil, "import-test")
	s.SetMetrics(m)
	resp, err := s.Import(ctx, "missing.xlsx", importWorkbook(t, [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", 2}}))
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.GetImport(ctx, resp.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(task.FilePath); err != nil {
		t.Fatal(err)
	}
	if err := s.processImport(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetImportResponse(ctx, task.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != string(model.ImportFailed) || got.SuccessRows != 0 || !strings.Contains(got.ErrorMsg, "源文件已丢失") || strings.Contains(got.ErrorMsg, task.FilePath) {
		t.Fatalf("missing file result: %+v", got)
	}
	stored, err := s.GetImport(ctx, task.TaskID)
	if err != nil || stored.RunToken != "" {
		t.Fatalf("execution not finished: %+v %v", stored, err)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.ReceiptOrder{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("missing file created orders: %d %v", count, err)
	}
	next, err := s.repo.NextPendingImport(context.Background(), db)
	if !errors.Is(err, gorm.ErrRecordNotFound) || next != nil {
		t.Fatalf("failed task stayed pending: %+v %v", next, err)
	}
	assertImportMetric(t, m, "wms_import_run_duration_seconds", "failed")
}

// The child stops inside the second row's transaction, after the first row has
// committed. Process.Kill bypasses defers, token release and graceful shutdown.
func TestImportRecoversAfterRealProcessKill(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing_source_%v", missing), func(t *testing.T) {
			s, db, ctx := importFixture(t)
			data := importWorkbook(t, [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", 2}, {"WH", "SKU", 3}, {"WH", "SKU", 4}})
			resp, err := s.Import(ctx, "crash.xlsx", data)
			if err != nil {
				t.Fatal(err)
			}
			var database string
			if err := db.Raw("SELECT DATABASE()").Scan(&database).Error; err != nil {
				t.Fatal(err)
			}
			cfg, err := mysqldriver.ParseDSN(os.Getenv("WMS_TEST_DSN"))
			if err != nil {
				t.Fatal(err)
			}
			cfg.DBName = database
			cfg.ParseTime = true
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			childCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(childCtx, executable, "-test.run=^TestImportCrashHelper$") // #nosec G204 -- executable is this test binary from os.Executable; arguments are fixed.
			cmd.Env = append(os.Environ(), "WMS_IMPORT_CRASH_HELPER=1", "WMS_IMPORT_CRASH_DSN="+cfg.FormatDSN())
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			waited := false
			defer func() {
				if !waited {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			}()
			ready := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					if scanner.Text() == "IMPORT_SECOND_ROW_BLOCKED" {
						ready <- true
						return
					}
				}
				ready <- false
			}()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("child exited before committing its first row")
				}
			case <-childCtx.Done():
				t.Fatal("child did not reach crash checkpoint")
			}
			first, err := s.repo.GetByImportRow(ctx, db, resp.TaskID, 2)
			if err != nil {
				t.Fatalf("first row was not committed before kill: %v", err)
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err == nil {
				t.Fatal("helper exited gracefully instead of being killed")
			}
			waited = true
			task, err := s.GetImport(ctx, resp.TaskID)
			if err != nil || task.Status != model.ImportProcessing || task.RunToken == "" {
				t.Fatalf("crash state: %+v %v", task, err)
			}
			m := observability.New(nil, "recovery-test")
			s.SetMetrics(m)
			s.compensateOnce(context.Background())
			fresh, err := s.GetImport(ctx, task.TaskID)
			if err != nil || fresh.RunToken != task.RunToken || fresh.Status != model.ImportProcessing {
				t.Fatalf("fresh lease was reset: %+v %v", fresh, err)
			}
			// Advance only the persisted heartbeat age, avoiding a five-minute test.
			if err := db.WithContext(ctx).Model(task).UpdateColumn("updated_at", time.Now().Add(-processingStaleThreshold-time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
			if missing {
				if err := os.Remove(task.FilePath); err != nil {
					t.Fatal(err)
				}
			}
			s.compensateOnce(context.Background())
			pending, err := s.repo.NextPendingImport(context.Background(), db)
			if err != nil || pending.TaskID != task.TaskID {
				t.Fatalf("crashed task not recovered: %+v %v", pending, err)
			}
			if err := s.processImport(context.Background(), pending); err != nil {
				t.Fatal(err)
			}
			got, err := s.GetImport(ctx, task.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			wantCount := int64(3)
			if missing {
				wantCount = 1
				if got.Status != model.ImportFailed || !strings.Contains(got.ErrorMsg, "核对已生成单据") {
					t.Fatalf("missing recovery: %+v", got)
				}
			} else if got.Status != model.ImportCompleted || got.SuccessRows != 3 || got.TotalRows != 3 || got.FailRows != 0 || got.FilePath != "" {
				t.Fatalf("resumed task: %+v", got)
			}
			var orders []model.ReceiptOrder
			if err := db.WithContext(ctx).Where("import_task_id = ?", task.TaskID).Order("import_row").Find(&orders).Error; err != nil {
				t.Fatal(err)
			}
			if int64(len(orders)) != wantCount || orders[0].ID != first.ID {
				t.Fatalf("recovery duplicated/replaced committed row: %+v", orders)
			}
			for i, order := range orders {
				if order.ImportRow != i+2 || order.ExpectedQty != i+2 {
					t.Fatalf("row mismatch: %+v", order)
				}
			}
			// The dead owner's token cannot overwrite the recovered result.
			if ok, err := s.repo.FinishImport(db.WithContext(ctx), task, model.ImportFailed, 0, 0, 1, "old"); err != nil || ok {
				t.Fatalf("old owner finished: %v %v", ok, err)
			}
			assertImportMetric(t, m, "wms_import_recoveries_total", "")
			outcome := "completed"
			if missing {
				outcome = "failed"
			}
			assertImportMetric(t, m, "wms_import_run_duration_seconds", outcome)
		})
	}
}

func TestImportCrashHelper(t *testing.T) {
	if os.Getenv("WMS_IMPORT_CRASH_HELPER") != "1" {
		return
	}
	dsn := os.Getenv("WMS_IMPORT_CRASH_DSN")
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(cfg.DBName, "gowms_test_") {
		t.Fatal("helper requires an isolated test schema")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	tm := tx.New(db)
	basic := basicservice.New(basicrepo.New(), tm, nil, nil, config.LimitsConfig{})
	// The dedicated schema contains one pending task; no task identifier or file
	// path from the child environment is passed into the business lookup.
	task, err := repository.New().NextPendingImport(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	s := New(repository.New(), tm, orderno.New(nil), basic, nil, nil, config.UploadConfig{Dir: filepath.Dir(task.FilePath)}, config.LimitsConfig{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:crash_checkpoint", func(call *gorm.DB) {
		order, ok := call.Statement.Dest.(*model.ReceiptOrder)
		if ok && order.ImportRow == 3 {
			fmt.Println("IMPORT_SECOND_ROW_BLOCKED")
			select {} // Parent force-kills this process; no cleanup may run.
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.processImport(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	t.Fatal("child unexpectedly completed without reaching the crash checkpoint")
}
