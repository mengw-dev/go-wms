package app

import (
	"fmt"
	"testing"
	"time"

	basicmodel "gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	outrepo "gowms/internal/modules/outbound/repository"
	outservice "gowms/internal/modules/outbound/service"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
)

func scaleLimitedOutbound(s *orderReferenceStack, limits config.LimitsConfig) *outservice.Service {
	tm := tx.New(s.db)
	return outservice.New(outrepo.New(), tm, orderno.New(nil), s.basic,
		invservice.New(invrepo.New(), tm), taskservice.New(taskrepo.New(), s.db), limits)
}

func requireScaleError(t *testing.T, err error, expected *errcode.Error) {
	t.Helper()
	if got := errcode.From(err); got == nil || got.Code != expected.Code {
		t.Fatalf("error=%v code=%v want=%d", err, got, expected.Code)
	}
}

func TestOutboundAllocationBudgetRollback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		batches []int
		limit   int
		pass    bool
	}{
		{"single_detail_over", []int{4}, 3, false},
		{"cumulative_over", []int{2, 2}, 3, false},
		{"no_budget_for_next_detail", []int{1, 1}, 1, false},
		{"exact_budget", []int{2, 1}, 3, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newOrderReferenceStack(t)
			items := make([]outdto.OrderDetailItem, 0, len(tc.batches))
			total := 0
			for i, batches := range tc.batches {
				skuID := snowflake.Next()
				if err := s.db.WithContext(s.ctx).Create(&basicmodel.SKU{Base: modelbase.Base{ID: skuID}, Code: fmt.Sprintf("LIMIT-%d", i), Barcode: fmt.Sprintf("LIMIT-BC-%d", i), Name: "limit", Status: 1}).Error; err != nil {
					t.Fatal(err)
				}
				items = append(items, outdto.OrderDetailItem{SKUID: skuID, ExpectedQty: batches})
				for j := range batches {
					locationID := snowflake.Next()
					for _, row := range []any{
						&basicmodel.Location{Base: modelbase.Base{ID: locationID}, WarehouseID: s.warehouseID, Code: fmt.Sprintf("L-%d-%d", i, j), Status: 1},
						&invmodel.Inventory{Base: modelbase.Base{ID: snowflake.Next()}, WarehouseID: s.warehouseID, LocationID: locationID, SKUID: skuID, BatchNo: "LIMIT", StockQuantity: 1, AvailableQty: 1, StockInTime: time.Unix(int64(j+1), 0)},
					} {
						if err := s.db.WithContext(s.ctx).Create(row).Error; err != nil {
							t.Fatal(err)
						}
					}
				}
				total += batches
			}
			order, err := s.out.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "LIMIT", Details: items}, "limit")
			if err != nil {
				t.Fatal(err)
			}
			if err := s.out.Submit(s.ctx, order.ID); err != nil {
				t.Fatal(err)
			}
			limited := scaleLimitedOutbound(s, config.LimitsConfig{MaxAllocationRows: tc.limit})
			attempts := 2 // rejection retry must not leave partial stock, tasks or flows.
			if tc.pass {
				attempts = 1
			}
			for range attempts {
				err = limited.Approve(s.ctx, order.ID, "limit")
				if tc.pass {
					if err != nil {
						t.Fatal(err)
					}
				} else {
					requireScaleError(t, err, errcode.AllocationScaleExceeded)
				}
				wantRows := int64(0)
				if tc.pass {
					wantRows = int64(total)
				}
				for _, row := range []any{&outmodel.Allocation{}, &taskmodel.Task{}, &invmodel.InventoryTrans{}} {
					var count int64
					if err := s.db.WithContext(s.ctx).Model(row).Count(&count).Error; err != nil || count != wantRows {
						t.Fatalf("%T count=%d want=%d err=%v", row, count, wantRows, err)
					}
				}
				var bad int64
				condition := "stock_quantity <> available_quantity + allocated_quantity OR available_quantity < 0 OR allocated_quantity < 0"
				if !tc.pass {
					condition += " OR allocated_quantity <> 0 OR available_quantity <> 1"
				}
				if err := s.db.WithContext(s.ctx).Model(&invmodel.Inventory{}).Where(condition).Count(&bad).Error; err != nil || bad != 0 {
					t.Fatalf("bad inventory=%d err=%v", bad, err)
				}
				var persisted outmodel.ShipmentOrder
				if err := s.db.WithContext(s.ctx).First(&persisted, order.ID).Error; err != nil {
					t.Fatal(err)
				}
				wantStatus := outmodel.OrderSubmitted
				if tc.pass {
					wantStatus = outmodel.OrderPicking
				}
				if persisted.Status != wantStatus {
					t.Fatalf("status=%s", persisted.Status)
				}
				if !tc.pass {
					if err := s.db.WithContext(s.ctx).Model(&outmodel.ShipmentOrderDetail{}).Where("allocated_qty <> 0").Count(&bad).Error; err != nil || bad != 0 {
						t.Fatalf("partial details=%d err=%v", bad, err)
					}
				}
			}
			// Raising the budget after rejection must allow a clean retry.
			if !tc.pass {
				if err := s.out.Approve(s.ctx, order.ID, "limit"); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.out.Cancel(s.ctx, order.ID, "limit"); err != nil {
				t.Fatal(err)
			}
			var bad int64
			if err := s.db.WithContext(s.ctx).Model(&invmodel.Inventory{}).Where("allocated_quantity <> 0 OR available_quantity <> 1").Count(&bad).Error; err != nil || bad != 0 {
				t.Fatalf("cancel bad=%d err=%v", bad, err)
			}
		})
	}
}

func TestOutboundDetailScaleAndExternalReplay(t *testing.T) {
	s := newOrderReferenceStack(t)
	second := snowflake.Next()
	if err := s.db.WithContext(s.ctx).Create(&basicmodel.SKU{Base: modelbase.Base{ID: second}, Code: "SECOND", Barcode: "SECOND-BC", Name: "second", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	limited := scaleLimitedOutbound(s, config.LimitsConfig{MaxShipmentDetails: 1})
	items := []outdto.OrderDetailItem{{SKUID: s.skuID, ExpectedQty: 1}, {SKUID: second, ExpectedQty: 1}}
	_, err := limited.Create(s.ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: "TOO-MANY", Details: items}, "limit")
	requireScaleError(t, err, errcode.ShipDetailScaleExceeded)
	external := &outdto.ExternalCreateOrderReq{WarehouseCode: "ORDER-WH", BizOrderNo: "EXTERNAL-LIMIT", Details: []outdto.ExternalOrderDetailItem{{SKUCode: "ORDER-SKU", ExpectedQty: 1}, {SKUCode: "SECOND", ExpectedQty: 1}}}
	_, _, err = limited.CreateExternal(s.ctx, external, "limit")
	requireScaleError(t, err, errcode.ShipDetailScaleExceeded)
	// Existing orders remain readable/replayable when operators lower the limit.
	order, replay, err := s.out.CreateExternal(s.ctx, external, "limit")
	if err != nil || replay {
		t.Fatalf("create err=%v replay=%v", err, replay)
	}
	got, replay, err := limited.CreateExternal(s.ctx, external, "limit")
	if err != nil || !replay || got.ID != order.ID {
		t.Fatalf("replay err=%v replay=%v", err, replay)
	}
	oversized := *external
	oversized.Details = append(append([]outdto.ExternalOrderDetailItem{}, external.Details...), outdto.ExternalOrderDetailItem{SKUCode: "MUST-NOT-RESOLVE", ExpectedQty: 1})
	_, _, err = limited.CreateExternal(s.ctx, &oversized, "limit")
	requireScaleError(t, err, errcode.ShipDetailScaleExceeded)
	if err := s.out.Submit(s.ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	requireScaleError(t, limited.Approve(s.ctx, order.ID, "limit"), errcode.ShipDetailScaleExceeded)
	var flows int64
	if err := s.db.WithContext(s.ctx).Model(&invmodel.InventoryTrans{}).Count(&flows).Error; err != nil || flows != 0 {
		t.Fatalf("flows=%d err=%v", flows, err)
	}
	if err := limited.Cancel(s.ctx, order.ID, "limit"); err != nil {
		t.Fatal(err)
	}
}
