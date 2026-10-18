package app

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
)

// Fault injection runs against the actual MySQL service chain. The first read
// misses normally; only the post-rollback verification is made unavailable.
func failSecondIdempotencyRead(t *testing.T, db *gorm.DB) *int {
	t.Helper()
	reads := 0
	const name = "test:reconcile_read_failure"
	if err := db.Callback().Query().Before("gorm:query").Register(name, func(q *gorm.DB) {
		if q.Statement.Table == "wms_idempotency" {
			reads++
			if reads == 2 {
				_ = q.AddError(errors.New("injected verification outage"))
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(name) })
	return &reads
}
func assertUnknownResult(t *testing.T, err error, reads int) {
	t.Helper()
	if reads != 2 {
		t.Fatalf("expected initial read and reconciliation, got %d", reads)
	}
	if err == nil || errcode.From(err).Code != errcode.IdempotencyResultUnknown.Code || errcode.From(err).HTTPStatus != 503 {
		t.Fatalf("failed verification must return unknown result, got %v", err)
	}
}

func TestPickReconcileFailurePreservesUnknownResult(t *testing.T) {
	st := newPickRaceStack(t)
	_, task := st.newPickOrder(t, "UNKNOWN-PICK", 2)
	reads := failSecondIdempotencyRead(t, st.db)
	_, err := st.out.Pick(st.ctx, task.ID, 999, "tester", strictScan(task), "", "unknown-pick")
	assertUnknownResult(t, err, *reads)
}
func TestClaimReconcileFailurePreservesUnknownResult(t *testing.T) {
	st := newPickRaceStack(t)
	_, task := st.newPickOrder(t, "UNKNOWN-CLAIM", 2)
	if _, err := st.out.ClaimPickTask(st.ctx, task.ID, "owner", "owned"); err != nil {
		t.Fatal(err)
	}
	reads := failSecondIdempotencyRead(t, st.db)
	_, err := st.out.ClaimPickTask(st.ctx, task.ID, "other", "unknown-claim")
	assertUnknownResult(t, err, *reads)
}
