package tx

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
)

func TestErrorClassification(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		retry, duplicate bool
	}{
		{"nil", nil, false, false},
		{"deadlock", &mysql.MySQLError{Number: 1213}, true, false},
		{"wrapped deadlock", fmt.Errorf("update inventory: %w", &mysql.MySQLError{Number: 1213}), true, false},
		{"lock timeout", &mysql.MySQLError{Number: 1205}, false, false},
		{"business conflict", fmt.Errorf("pick: %w", errcode.ShipOrderVersionBad), true, false},
		{"duplicate key", &mysql.MySQLError{Number: 1062}, false, true},
		{"wrapped duplicate", fmt.Errorf("insert: %w", &mysql.MySQLError{Number: 1062}), false, true},
		{"translated duplicate", fmt.Errorf("insert: %w", gorm.ErrDuplicatedKey), false, true},
		{"misleading text", errors.New("duplicate request with deadlock in description"), false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.retry {
				t.Errorf("IsRetryable = %v, want %v", got, tt.retry)
			}
			if got := IsDuplicateErr(tt.err); got != tt.duplicate {
				t.Errorf("IsDuplicateErr = %v, want %v", got, tt.duplicate)
			}
		})
	}
}

func TestTxRetryRejectsInvalidAttemptsAndCanceledContext(t *testing.T) {
	m := New(nil)
	fn := func(*gorm.DB) error { t.Fatal("transaction must not start"); return nil }
	for _, attempts := range []int{-1, 0} {
		if err := m.TxRetry(context.Background(), attempts, fn); err == nil {
			t.Fatalf("attempts %d: expected error", attempts)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.TxRetry(ctx, 3, fn); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want cancellation", err)
	}
}
