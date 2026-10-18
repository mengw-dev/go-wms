package app

import (
	"context"
	"testing"
	"time"

	"gowms/internal/pkg/errcode"
)

// A rejected claim must not borrow a second connection while holding the only
// transaction connection. It must also leave the key available for retry.
func TestRejectedClaimWithSingleConnection(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "IDEM-CLAIM-POOL", 2)
	if _, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "owner", "owner-key"); err != nil {
		t.Fatal(err)
	}
	pool, err := stack.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	pool.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(stack.ctx, 3*time.Second)
	defer cancel()
	if _, err := stack.out.ClaimPickTask(ctx, task.ID, "other", "rejected-key"); errcode.From(err).Code != errcode.TaskClaimConflict.Code {
		t.Fatalf("expected claim conflict without waiting for another connection, got %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("claim exhausted deadline: %v", ctx.Err())
	}
	if _, err := stack.out.ClaimPickTask(ctx, task.ID, "owner", "owner-key"); err != nil {
		t.Fatalf("original claim must still replay: %v", err)
	}
}
