package service

import "testing"

func TestSummarizeConcurrentAttemptsByStage(t *testing.T) {
	attempts := []concurrentAttempt{
		{OrderID: 101, Created: true, Submitted: true, Approved: true},
		{OrderID: 102, Created: true, Submitted: false, FailedPhase: "submit", FailureCause: "submit failed"},
		{OrderID: 103, Created: true, Submitted: true, Approved: false, FailedPhase: "approve", FailureCause: "stock unavailable"},
		{FailedPhase: "create", FailureCause: "duplicate order"},
	}

	got := summarizeConcurrentAttempts(attempts)
	if got.CreatedOrders != 3 || got.SubmittedOrders != 2 || got.ApprovedOrders != 1 {
		t.Fatalf("stage counts = created:%d submitted:%d approved:%d", got.CreatedOrders, got.SubmittedOrders, got.ApprovedOrders)
	}
	if got.ApprovalFailed != 1 || got.OtherFailed != 2 {
		t.Fatalf("failure counts = approval:%d other:%d", got.ApprovalFailed, got.OtherFailed)
	}
	if len(got.ApprovedOrderIDs) != 1 || got.ApprovedOrderIDs[0] != 101 {
		t.Fatalf("approved order ids = %#v", got.ApprovedOrderIDs)
	}
	if len(got.FailureReasons) != 3 {
		t.Fatalf("failure reasons = %#v", got.FailureReasons)
	}
}

func TestConcurrentInventoryInvariant(t *testing.T) {
	if !concurrentInventoryInvariantOK(&concurrentInventoryStats{StockTotal: 100, AvailableTotal: 60, AllocatedTotal: 40}) {
		t.Fatal("valid stock equation should pass")
	}
	if concurrentInventoryInvariantOK(&concurrentInventoryStats{StockTotal: 100, AvailableTotal: 70, AllocatedTotal: 40}) {
		t.Fatal("broken stock equation should fail")
	}
	if concurrentInventoryInvariantOK(&concurrentInventoryStats{StockTotal: 100, AvailableTotal: 100, AllocatedTotal: 0, NegativeRows: 1}) {
		t.Fatal("negative rows should fail")
	}
}
