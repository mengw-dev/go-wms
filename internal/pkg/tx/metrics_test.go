package tx

import (
	"context"
	"errors"
	"testing"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/observability"
)

func TestTransactionAttemptAndActualRetryMetrics(t *testing.T) {
	db := retryDatabase(t)
	m := observability.New(nil, "transaction-test")
	mgr := New(db)
	mgr.SetMetrics(m)
	ctx := context.Background()
	attempt := 0
	err := mgr.TxRetry(ctx, 3, func(*gorm.DB) error {
		attempt++
		if attempt == 1 {
			return &mysql.MySQLError{Number: 1213}
		}
		return nil
	})
	if err != nil || attempt != 2 {
		t.Fatalf("retry result: %d %v", attempt, err)
	}
	if err := mgr.TxRetry(ctx, 3, func(*gorm.DB) error { return &mysql.MySQLError{Number: 1205} }); errcode.From(err).Code != errcode.TransactionBusy.Code {
		t.Fatalf("lock timeout=%v", err)
	}
	if err := mgr.TxRetry(ctx, 3, func(*gorm.DB) error { return errcode.OrderVersionBad }); err == nil {
		t.Fatal("expected exhausted conflict")
	}
	canceled, cancel := context.WithCancel(ctx)
	if err := mgr.TxRetry(canceled, 3, func(*gorm.DB) error { cancel(); return errcode.OrderVersionBad }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("transaction swallowed panic")
			}
		}()
		_ = mgr.Tx(ctx, func(*gorm.DB) error { panic("test rollback") })
	}()
	families, err := m.Registry().Gather()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "wms_transaction_attempt_duration_seconds" && family.GetName() != "wms_transaction_retries_total" {
			continue
		}
		for _, metric := range family.Metric {
			if len(metric.Label) != 1 {
				t.Fatal("transaction metric contains identifier labels")
			}
			key := family.GetName() + "/" + metric.Label[0].GetValue()
			if metric.Histogram != nil {
				got[key] = float64(metric.GetHistogram().GetSampleCount())
			} else {
				got[key] = metric.GetCounter().GetValue()
			}
		}
	}
	want := map[string]float64{
		"wms_transaction_attempt_duration_seconds/deadlock":     1,
		"wms_transaction_attempt_duration_seconds/success":      1,
		"wms_transaction_attempt_duration_seconds/lock_timeout": 1,
		"wms_transaction_attempt_duration_seconds/conflict":     4,
		"wms_transaction_attempt_duration_seconds/panic":        1,
		"wms_transaction_retries_total/deadlock":                1,
		"wms_transaction_retries_total/conflict":                2,
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected series: %v", got)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s=%v want %v", key, got[key], value)
		}
	}
}
