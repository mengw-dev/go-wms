package service

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/inbound/dto"
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

func TestReceiveReconcileFailurePreservesUnknownResult(t *testing.T) {
	st := newReceiveIdemStack(t, 2)
	reads := failSecondIdempotencyRead(t, st.db)
	err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, &dto.ReceiveReq{Qty: 3, BatchNo: "B"}, "tester", "unknown-receive")
	assertUnknownResult(t, err, *reads)
}
func TestPutawayReconcileFailurePreservesUnknownResult(t *testing.T) {
	st := newPutawayIdemStack(t)
	reads := failSecondIdempotencyRead(t, st.db)
	err := st.s.Putaway(st.ctx, st.task.ID, st.location, 999, "tester", "unknown-putaway")
	assertUnknownResult(t, err, *reads)
}
