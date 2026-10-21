package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/model"
	"gowms/internal/pkg/observability"
	"gowms/internal/pkg/tenant"
)

func TestImportMetricsForPartialAndGracefulRelease(t *testing.T) {
	for _, release := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "release"}[release], func(t *testing.T) {
			s, db, ctx := importFixture(t)
			m := observability.New(nil, "run-test")
			s.SetMetrics(m)
			resp, err := s.Import(ctx, "input.xlsx", importWorkbook(t, [][]any{{"仓库编码", "货品编码", "预期数量"}, {"WH", "SKU", 2}, {"WH", "SKU", "bad"}}))
			if err != nil {
				t.Fatal(err)
			}
			task, err := s.GetImport(ctx, resp.TaskID)
			if err != nil {
				t.Fatal(err)
			}
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			if release {
				if err := db.Callback().Create().Before("gorm:create").Register("test:cancel_import", func(call *gorm.DB) {
					if _, ok := call.Statement.Dest.(*model.ReceiptOrder); ok {
						cancel()
					}
				}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.processImport(parent, task); err != nil {
				t.Fatal(err)
			}
			outcome := "partial"
			wantStatus := model.ImportCompleted
			if release {
				outcome = "released"
				wantStatus = model.ImportPending
			}
			assertImportMetric(t, m, "wms_import_run_duration_seconds", outcome)
			stored, err := s.GetImport(ctx, task.TaskID)
			if err != nil || stored.Status != wantStatus || stored.RunToken != "" {
				t.Fatalf("run result: %+v %v", stored, err)
			}
		})
	}
}

func assertImportMetric(t *testing.T, m *observability.Metrics, name, outcome string) {
	t.Helper()
	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.Metric {
			matched := outcome == ""
			for _, label := range metric.Label {
				if label.GetName() == "outcome" && label.GetValue() == outcome {
					matched = true
				}
			}
			if !matched {
				continue
			}
			got := metric.GetCounter().GetValue()
			if metric.Histogram != nil {
				got = float64(metric.GetHistogram().GetSampleCount())
			}
			if got != 1 {
				t.Fatalf("%s/%s=%v want 1", name, outcome, got)
			}
			return
		}
	}
	t.Fatalf("metric %s/%s missing", name, outcome)
}

func TestImportQueueMetricsAcrossTenantsAndFailure(t *testing.T) {
	s, db, _ := importFixture(t)
	metrics := observability.New(nil, "queue-test")
	s.SetMetrics(metrics)
	statuses := []model.ImportTaskStatus{model.ImportPending, model.ImportPending, model.ImportProcessing, model.ImportFailed, model.ImportCompleted, model.ImportPending}
	for i, status := range statuses {
		task := &model.ImportTask{TaskID: string(rune('a' + i)), Status: status}
		task.ID = int64(100 + i)
		task.CreatedAt = time.Now().Add(-10 * time.Minute)
		if err := db.WithContext(tenant.WithExactTenant(context.Background(), int64(i))).Create(task).Error; err != nil {
			t.Fatal(err)
		}
		if i == 5 {
			if err := db.Delete(task).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// Monitoring must aggregate tenant 0 and ordinary tenants, excluding soft deletion.
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, family := range families {
		switch family.GetName() {
		case "wms_import_queue_tasks":
			if len(family.Metric) != 2 {
				t.Fatal("unbounded status labels")
			}
			for _, metric := range family.Metric {
				if len(metric.Label) != 1 || metric.Label[0].GetName() != "status" {
					t.Fatal("queue leaked identifier labels")
				}
				status := metric.Label[0].GetValue()
				want := 1.0
				if status == "PENDING" {
					want = 2
				}
				if metric.GetGauge().GetValue() != want {
					t.Fatalf("%s count=%v", status, metric.GetGauge().GetValue())
				}
				seen[status] = true
			}
		case "wms_import_oldest_pending_age_seconds":
			age := family.Metric[0].GetGauge().GetValue()
			if age < 590 || age > 620 {
				t.Fatalf("oldest pending age=%v", age)
			}
			seen["age"] = true
		case "wms_import_queue_scrape_success":
			if family.Metric[0].GetGauge().GetValue() != 1 {
				t.Fatal("scrape failed")
			}
		}
	}
	if len(seen) != 3 {
		t.Fatalf("metrics missing: %v", seen)
	}
	if err := db.WithContext(context.Background()).Model(&model.ImportTask{}).Where("status = ?", model.ImportPending).Update("status", model.ImportCompleted).Error; err != nil {
		t.Fatal(err)
	}
	families, err = metrics.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() == "wms_import_oldest_pending_age_seconds" && family.Metric[0].GetGauge().GetValue() != 0 {
			t.Fatal("empty queue retained old age")
		}
	}
	// A database outage must not masquerade as an empty queue or remove other metrics.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	families, err = metrics.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	failureSeen := false
	for _, family := range families {
		if family.GetName() == "wms_import_queue_tasks" || family.GetName() == "wms_import_oldest_pending_age_seconds" {
			t.Fatal("failed scrape emitted false queue data")
		}
		if family.GetName() == "wms_import_queue_scrape_success" {
			failureSeen = family.Metric[0].GetGauge().GetValue() == 0
		}
	}
	if !failureSeen {
		t.Fatal("missing scrape failure signal")
	}
}
