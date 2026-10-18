package app

import (
	"errors"
	"testing"

	indto "gowms/internal/modules/inbound/dto"
	outdto "gowms/internal/modules/outbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/quantity"
)

func TestOrderQuantitySumsRejectOverflow(t *testing.T) {
	s := newOrderReferenceStack(t)
	for _, values := range [][]int{{quantity.Max + 1}, {quantity.Max, 1}, {0}, {-1}} {
		inRows, outRows := []indto.OrderDetailItem{}, []outdto.OrderDetailItem{}
		for i, qty := range values {
			inRows = append(inRows, indto.OrderDetailItem{SKUID: s.skuID + int64(i), ExpectedQty: qty})
			outRows = append(outRows, outdto.OrderDetailItem{SKUID: s.skuID + int64(i), ExpectedQty: qty})
		}
		if _, err := s.in.Create(s.ctx, &indto.CreateOrderReq{WarehouseID: s.warehouseID, Details: inRows}, "test"); !errors.Is(err, errcode.QuantityLimitExceeded) {
			t.Fatalf("inbound %v: %v", values, err)
		}
		if _, err := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "OVERFLOW", Details: outRows}, "test"); !errors.Is(err, errcode.QuantityLimitExceeded) {
			t.Fatalf("outbound %v: %v", values, err)
		}
	}
}
