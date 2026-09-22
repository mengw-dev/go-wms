package model

import "testing"

func TestStocktakeStatusTransitions(t *testing.T) {
	tests := []struct {
		from OrderStatus
		to   OrderStatus
		want bool
	}{
		{OrderDraft, OrderCompleted, true},
		{OrderDraft, OrderCancelled, true},
		{OrderCompleted, OrderDraft, false},
		{OrderCompleted, OrderCancelled, false},
		{OrderCancelled, OrderDraft, false},
		{OrderCancelled, OrderCompleted, false},
	}
	for _, tt := range tests {
		if got := CanTransit(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransit(%s, %s)=%v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}
