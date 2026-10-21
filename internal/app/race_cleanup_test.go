package app

import (
	"context"
	"testing"
	"time"
)

// Returning before releasing a barrier models an early assertion exit. The
// fixture must release/cancel and join its worker before dropping its database.
func TestPickRaceFixtureCleansUnreleasedGate(t *testing.T) {
	finished := make(chan struct{})
	t.Run("leave_gate_held", func(t *testing.T) {
		stack := newPickRaceStack(t)
		_, task := stack.newPickOrder(t, "CLEANUP-GATE", 2)
		gate := newTaskUpdateGate(t, stack.db, "cleanup:unreleased")
		stack.run(func() {
			defer close(finished)
			_, _ = stack.out.Pick(stack.ctx, task.ID, 1, "picker", nil, "", "")
		})
		waitClosed(t, "worker reached gate", gate.arrived)
		// No explicit open or worker wait: testing cleanup owns both.
	})
	select {
	case <-finished:
	default:
		t.Fatal("fixture returned before its worker stopped")
	}
}

func TestTaskUpdateGateHonorsCancellation(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "CLEANUP-CANCEL", 2)
	gate := newTaskUpdateGate(t, stack.db, "cleanup:canceled")
	ctx, cancel := context.WithCancel(stack.ctx)
	defer cancel()
	done := make(chan struct{})
	stack.run(func() {
		defer close(done)
		_, _ = stack.out.Pick(ctx, task.ID, 1, "picker", nil, "", "")
	})
	waitClosed(t, "worker reached gate", gate.arrived)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("canceled worker stayed blocked on the test gate")
	}
	gate.open()
	gate.open() // explicit release and cleanup must be safe together.
}
