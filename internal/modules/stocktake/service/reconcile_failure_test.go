package service

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/stocktake/dto"
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

func TestApproveReconcileFailurePreservesUnknownResult(t *testing.T) {
	s, db, ctx, _ := stocktakeFixture(t)
	order, err := s.Create(ctx, &dto.CreateOrderReq{WarehouseID: 1}, "test")
	if err != nil {
		t.Fatal(err)
	}
	reads := failSecondIdempotencyRead(t, db)
	err = s.Approve(ctx, order.ID, "test", "unknown-approve")
	assertUnknownResult(t, err, *reads)
}
