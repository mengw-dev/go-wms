package idempotency

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/pkg/observability"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func cleanupFixture(t *testing.T, n int) (*gorm.DB, time.Time) {
	t.Helper()
	db := testutil.OpenIsolatedMySQL(t, os.Getenv("WMS_TEST_DSN"), &Record{})
	before := time.Now().Truncate(time.Second).Add(-RecordRetention)
	records := make([]Record, 0, n+3)
	for i := 1; i <= n+3; i++ {
		created := before.Add(-time.Hour)
		if i > n {
			created = before.Add(time.Duration(i-n-1) * time.Hour)
		}
		records = append(records, Record{ID: int64(i), TenantID: int64(i % 3), Scope: "cleanup-test", IdempotencyKey: fmt.Sprint(i), RequestHash: "hash", ResultJSON: "{}", CreatedAt: created})
	}
	if err := db.CreateInBatches(records, 200).Error; err != nil {
		t.Fatal(err)
	}
	// Unknown historical timestamps are not inferred to be expired.
	if err := db.Model(&Record{}).Where("id = ?", n+3).UpdateColumn("created_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	return db, before
}

func remainingExpired(t *testing.T, db *gorm.DB, before time.Time) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&Record{}).Where("created_at < ?", before).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPurgeBoundedBatchesRetainBoundaryFreshAndUnknown(t *testing.T) {
	const expired = CleanupBatchSize*2 + 7
	db, before := cleanupFixture(t, expired)
	var sizes []int64
	if err := db.Callback().Delete().After("gorm:delete").Register("test:bounded_deletes", func(call *gorm.DB) {
		if call.Statement.Table == "wms_idempotency" {
			sizes = append(sizes, call.RowsAffected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	result, err := purge(db, before)
	if err != nil || result.rows != expired || result.batches != 3 || result.limited {
		t.Fatalf("purge=%+v %v", result, err)
	}
	if len(sizes) != 3 || sizes[0] != CleanupBatchSize || sizes[1] != CleanupBatchSize || sizes[2] != 7 {
		t.Fatalf("delete sizes=%v", sizes)
	}
	var remaining []Record
	if err := db.Order("id").Find(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 3 || !remaining[0].CreatedAt.Equal(before) || remaining[1].CreatedAt.Before(before) || !remaining[2].CreatedAt.IsZero() {
		t.Fatalf("retention boundary changed: %+v", remaining)
	}
	rows, err := Purge(db, before)
	if err != nil || rows != 0 {
		t.Fatalf("second cleanup=%d %v", rows, err)
	}
}

func TestPurgeCapAndResume(t *testing.T) {
	const expired = CleanupBatchSize*CleanupMaxBatches + 1
	db, before := cleanupFixture(t, expired)
	result, err := purge(db, before)
	if err != nil || result.rows != expired-1 || result.batches != CleanupMaxBatches || !result.limited {
		t.Fatalf("capped cleanup=%+v %v", result, err)
	}
	if remainingExpired(t, db, before) != 1 {
		t.Fatal("round cap did not bound deletion")
	}
	rows, err := Purge(db, before)
	if err != nil || rows != 1 || remainingExpired(t, db, before) != 0 {
		t.Fatalf("resume=%d %v", rows, err)
	}
}

func TestPurgeTenantScopeIncludingExactZero(t *testing.T) {
	db, before := cleanupFixture(t, 9)
	for _, id := range []int64{0, 1} {
		rows, err := Purge(db.WithContext(tenant.WithExactTenant(context.Background(), id)), before)
		if err != nil || rows != 3 {
			t.Fatalf("tenant %d rows=%d %v", id, rows, err)
		}
	}
	if remainingExpired(t, db, before) != 3 {
		t.Fatal("scoped cleanup crossed tenants")
	}
	rows, err := Purge(db, before)
	if err != nil || rows != 3 {
		t.Fatalf("platform cleanup=%d %v", rows, err)
	}
}

func TestPurgeFailurePreservesCommittedProgress(t *testing.T) {
	db, before := cleanupFixture(t, CleanupBatchSize+7)
	injected := errors.New("injected second delete failure")
	deletes := 0
	if err := db.Callback().Delete().Before("gorm:delete").Register("test:fail_second", func(call *gorm.DB) {
		deletes++
		if deletes == 2 {
			_ = call.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}
	result, err := purge(db, before)
	if !errors.Is(err, injected) || result.rows != CleanupBatchSize || result.batches != 1 || remainingExpired(t, db, before) != 7 {
		t.Fatalf("partial cleanup=%+v %v", result, err)
	}
	if err := db.Callback().Delete().Remove("test:fail_second"); err != nil {
		t.Fatal(err)
	}
	rows, err := Purge(db, before)
	if err != nil || rows != 7 {
		t.Fatalf("resume=%d %v", rows, err)
	}
}

func TestPurgeConcurrentCleanersCountOnlyTheirDeletes(t *testing.T) {
	const expired = CleanupBatchSize + 9
	db, before := cleanupFixture(t, expired)
	var arrivals atomic.Int32
	gate := make(chan struct{})
	if err := db.Callback().Query().After("gorm:query").Register("test:same_selection", func(call *gorm.DB) {
		if call.Statement.Table != "wms_idempotency" {
			return
		}
		if _, ok := call.Statement.Dest.(*[]int64); !ok {
			return
		}
		switch arrivals.Add(1) {
		case 1:
			select {
			case <-gate:
			case <-call.Statement.Context.Done():
				_ = call.AddError(call.Statement.Context.Err())
			}
		case 2:
			close(gate)
		}
	}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make([]purgeResult, 2)
	errorsFound := make([]error, 2)
	for i := range 2 {
		wg.Go(func() { results[i], errorsFound[i] = purge(db, before) })
	}
	wg.Wait()
	for _, err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0].rows+results[1].rows != expired || remainingExpired(t, db, before) != 0 {
		t.Fatalf("concurrent progress=%+v", results)
	}
}

func TestPurgeBatchDeadlineAndParentDeadline(t *testing.T) {
	for _, parentLimit := range []time.Duration{0, time.Second} {
		t.Run(parentLimit.String(), func(t *testing.T) {
			db, before := cleanupFixture(t, CleanupBatchSize+1)
			holder := db.Begin()
			if holder.Error != nil {
				t.Fatal(holder.Error)
			}
			defer holder.Rollback()
			if err := holder.Exec("UPDATE wms_idempotency SET request_hash='held' WHERE id=1").Error; err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			cancel := func() {}
			if parentLimit > 0 {
				ctx, cancel = context.WithTimeout(ctx, parentLimit)
			}
			defer cancel()
			start := time.Now()
			result, err := purge(db.WithContext(ctx), before)
			if !errors.Is(err, context.DeadlineExceeded) || result.rows != 0 || result.batches != 0 {
				t.Fatalf("timed cleanup=%+v %v", result, err)
			}
			budget := CleanupBatchTimeout
			if parentLimit > 0 {
				budget = parentLimit
			}
			if time.Since(start) > budget+time.Second {
				t.Fatal("cleanup exceeded deadline bound")
			}
			if err := holder.Rollback().Error; err != nil {
				t.Fatal(err)
			}
			if remainingExpired(t, db, before) != CleanupBatchSize+1 {
				t.Fatal("timed out batch was partially committed")
			}
			if rows, err := Purge(db, before); err != nil || rows != CleanupBatchSize+1 {
				t.Fatalf("timeout resume=%d %v", rows, err)
			}
		})
	}
}

func TestPurgeRechecksExpirationAfterSelection(t *testing.T) {
	db, before := cleanupFixture(t, 1)
	if err := db.Callback().Query().After("gorm:query").Register("test:refresh_expiry", func(call *gorm.DB) {
		if call.Statement.Table != "wms_idempotency" {
			return
		}
		if _, ok := call.Statement.Dest.(*[]int64); !ok {
			return
		}
		if err := db.Model(&Record{}).Where("id=1").UpdateColumn("created_at", before).Error; err != nil {
			_ = call.AddError(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := Purge(db, before)
	if err != nil || rows != 0 {
		t.Fatalf("expiry recheck=%d %v", rows, err)
	}
	var record Record
	if err := db.First(&record, 1).Error; err != nil || !record.CreatedAt.Equal(before) {
		t.Fatalf("updated record deleted: %+v %v", record, err)
	}
}

func cleanupMetrics(t *testing.T, m *observability.Metrics) map[string]float64 {
	t.Helper()
	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	result := map[string]float64{}
	for _, family := range families {
		name := family.GetName()
		if name != "wms_idempotency_cleanup_duration_seconds" && name != "wms_idempotency_cleanup_deleted_rows_total" && name != "wms_idempotency_cleanup_batches_total" && name != "wms_idempotency_cleanup_last_success_timestamp_seconds" {
			continue
		}
		for _, metric := range family.Metric {
			key := name
			if len(metric.Label) > 1 {
				t.Fatal("unbounded cleanup labels")
			}
			if len(metric.Label) == 1 {
				key += "/" + metric.Label[0].GetValue()
			}
			switch {
			case metric.Counter != nil:
				result[key] = metric.GetCounter().GetValue()
			case metric.Gauge != nil:
				result[key] = metric.GetGauge().GetValue()
			case metric.Histogram != nil:
				result[key] = float64(metric.GetHistogram().GetSampleCount())
			}
		}
	}
	return result
}

func TestRunCleanupResumesCappedRoundAndStops(t *testing.T) {
	const expired = CleanupBatchSize*CleanupMaxBatches + 1
	db, before := cleanupFixture(t, expired)
	// Move exact-cutoff/fresh rows well into the future: scheduler cutoffs advance.
	if err := db.Model(&Record{}).Where("id > ? AND created_at IS NOT NULL", expired).UpdateColumn("created_at", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	metrics := observability.New(nil, "cleanup-test")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); RunCleanup(ctx, db, 20*time.Millisecond, RecordRetention, metrics) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("cleanup did not stop")
		}
	}()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		got := cleanupMetrics(t, metrics)
		if got["wms_idempotency_cleanup_last_success_timestamp_seconds"] > 0 {
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("worker did not stop")
			}
			got = cleanupMetrics(t, metrics)
			if got["wms_idempotency_cleanup_duration_seconds/limited"] != 1 || got["wms_idempotency_cleanup_deleted_rows_total"] != expired || got["wms_idempotency_cleanup_batches_total"] != CleanupMaxBatches+1 || remainingExpired(t, db, before) != 0 {
				t.Fatalf("cleanup metrics=%v", got)
			}
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("capped round was not resumed")
		case <-ticker.C:
		}
	}
}

func TestRunCleanupReportsPartialFailureTimeoutAndCancellation(t *testing.T) {
	for _, mode := range []string{"error", "query_error", "timeout", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			db, _ := cleanupFixture(t, CleanupBatchSize+1)
			if err := db.Model(&Record{}).Where("id > ? AND created_at IS NOT NULL", CleanupBatchSize+1).UpdateColumn("created_at", time.Now().Add(time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			deletes := 0
			if err := db.Callback().Delete().Before("gorm:delete").Register("test:failed_round", func(call *gorm.DB) {
				deletes++
				if deletes != 2 {
					return
				}
				switch mode {
				case "error":
					_ = call.AddError(errors.New("injected delete failure"))
				case "timeout":
					<-call.Statement.Context.Done()
					_ = call.AddError(call.Statement.Context.Err())
				case "canceled":
					cancel()
					_ = call.AddError(ctx.Err())
				}
			}); err != nil {
				t.Fatal(err)
			}
			if mode == "query_error" {
				if err := db.Callback().Query().Before("gorm:query").Register("test:failed_selection", func(call *gorm.DB) { _ = call.AddError(errors.New("injected select failure")) }); err != nil {
					t.Fatal(err)
				}
			}
			metrics := observability.New(nil, "failure-test")
			done := make(chan struct{})
			go func() { defer close(done); RunCleanup(ctx, db, CleanupInterval, RecordRetention, metrics) }()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Error("failed worker did not stop")
				}
			}()
			outcome := mode
			if mode == "query_error" {
				outcome = "error"
			}
			deadline := time.NewTimer(6 * time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for cleanupMetrics(t, metrics)["wms_idempotency_cleanup_duration_seconds/"+outcome] == 0 {
				select {
				case <-deadline.C:
					t.Fatal("round outcome not recorded")
				case <-ticker.C:
				}
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("cancel during retry wait did not stop")
			}
			got := cleanupMetrics(t, metrics)
			wantRows, wantBatches := float64(CleanupBatchSize), 1.0
			if mode == "query_error" {
				wantRows, wantBatches = 0, 0
			}
			if got["wms_idempotency_cleanup_deleted_rows_total"] != wantRows || got["wms_idempotency_cleanup_batches_total"] != wantBatches || got["wms_idempotency_cleanup_last_success_timestamp_seconds"] != 0 {
				t.Fatalf("failure metrics=%v", got)
			}
		})
	}
}

func TestCleanupDelayPolicyAndCanceledStartup(t *testing.T) {
	for _, tt := range []struct {
		limited bool
		err     error
		want    time.Duration
	}{
		{false, nil, CleanupInterval}, {true, nil, CleanupBacklogInterval}, {false, errors.New("failed"), CleanupBacklogInterval},
	} {
		if got := nextCleanupDelay(CleanupInterval, tt.limited, tt.err); got != tt.want {
			t.Fatalf("next delay=%v want %v", got, tt.want)
		}
	}
	if got := nextCleanupDelay(time.Second, true, nil); got != time.Second {
		t.Fatalf("short interval extended: %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	RunCleanup(ctx, nil, CleanupInterval, RecordRetention, nil)
}
