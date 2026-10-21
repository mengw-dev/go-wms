package observability_test

import (
	"context"
	"os"
	"testing"
	"time"

	"gowms/internal/pkg/observability"
	"gowms/internal/testutil"
)

func TestMySQLRowLockMetricsAndFailedScrape(t *testing.T) {
	db := testutil.OpenIsolatedMySQL(t, os.Getenv("WMS_TEST_DSN"))
	if err := db.Exec("CREATE TABLE metric_locks (id INT PRIMARY KEY, n INT NOT NULL) ENGINE=InnoDB").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO metric_locks VALUES (1, 0)").Error; err != nil {
		t.Fatal(err)
	}
	m := observability.New(db, "lock-test")
	gather := func() map[string]float64 {
		t.Helper()
		families, err := m.Registry().Gather()
		if err != nil {
			t.Fatal(err)
		}
		result := map[string]float64{}
		for _, family := range families {
			if len(family.Metric) != 1 {
				continue
			}
			metric := family.Metric[0]
			if metric.Gauge != nil {
				result[family.GetName()] = metric.GetGauge().GetValue()
			}
			if metric.Counter != nil {
				result[family.GetName()] = metric.GetCounter().GetValue()
			}
		}
		return result
	}
	before := gather()
	if before["wms_mysql_row_lock_scrape_success"] != 1 {
		t.Fatal("MySQL row lock status unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	holder := db.WithContext(ctx).Begin()
	if holder.Error != nil {
		t.Fatal(holder.Error)
	}
	defer holder.Rollback()
	if err := holder.Exec("UPDATE metric_locks SET n=n+1 WHERE id=1").Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- db.WithContext(ctx).Exec("UPDATE metric_locks SET n=n+1 WHERE id=1").Error }()
	defer func() {
		cancel()
		_ = holder.Rollback().Error
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("lock waiter did not stop")
		}
	}()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for gather()["wms_mysql_row_lock_current_waits"] < 1 {
		select {
		case <-ctx.Done():
			t.Fatal("real row lock wait was not observed")
		case <-ticker.C:
		}
	}
	if err := holder.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
		// Keep the cleanup receive non-blocking after successful completion.
		done <- nil
	case <-ctx.Done():
		t.Fatal("waiter failed after releasing row lock")
	}
	after := gather()
	if after["wms_mysql_row_lock_waits_total"] <= before["wms_mysql_row_lock_waits_total"] || after["wms_mysql_row_lock_wait_seconds_total"] < before["wms_mysql_row_lock_wait_seconds_total"] {
		t.Fatal("row lock cumulative counters did not advance")
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	failed := gather()
	if failed["wms_mysql_row_lock_scrape_success"] != 0 {
		t.Fatal("failed scrape reported success")
	}
	if _, exists := failed["wms_mysql_row_lock_current_waits"]; exists {
		t.Fatal("failed scrape emitted false zero")
	}
	if _, exists := failed["wms_db_wait_count_total"]; !exists {
		t.Fatal("connection pool metrics disappeared")
	}
}
