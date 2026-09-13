package model

import "testing"

// TestCanTransit 出库单状态机：合法流转全通过，非法/跨状态/终态流转全拒绝。
func TestCanTransit(t *testing.T) {
	legal := map[OrderStatus][]OrderStatus{
		OrderDraft:     {OrderSubmitted, OrderCancelled},
		OrderSubmitted: {OrderPicking, OrderCancelled},
		OrderApproved:  {OrderPicking, OrderCancelled},
		OrderPicking:   {OrderShipped, OrderCancelled},
	}
	for from, tos := range legal {
		for _, to := range tos {
			if !CanTransit(from, to) {
				t.Errorf("expect legal: %s -> %s", from, to)
			}
		}
	}

	illegal := []struct {
		from, to OrderStatus
	}{
		// 跨状态跳转
		{OrderDraft, OrderPicking},
		{OrderDraft, OrderShipped},
		{OrderSubmitted, OrderShipped},
		{OrderPicking, OrderSubmitted},
		// 终态无后继
		{OrderShipped, OrderPicking},
		{OrderShipped, OrderCancelled},
		{OrderCancelled, OrderDraft},
		{OrderCancelled, OrderShipped},
		// 回退
		{OrderPicking, OrderDraft},
	}
	for _, c := range illegal {
		if CanTransit(c.from, c.to) {
			t.Errorf("expect illegal: %s -> %s", c.from, c.to)
		}
	}
}
