package app

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	basicmodel "gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
	outdto "gowms/internal/modules/outbound/dto"
	outmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
)

// TestOutboundScaleProfile is a bounded service/SQL experiment, not an HTTP SLA.
// Full tests use one sample; WMS_SCALE_PROFILE=1 uses five committed approvals,
// each followed by business cancellation, in a private schema for each case.
func TestOutboundScaleProfile(t *testing.T) {
	for _, size := range []struct{ details, batches int }{{1, 1}, {50, 1}, {200, 1}, {1, 250}, {1, 1000}, {200, 5}} {
		t.Run(fmt.Sprintf("details%d_batches%d", size.details, size.batches), func(t *testing.T) {
			s := newOrderReferenceStack(t)
			ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 81), 60*time.Second)
			defer cancel()
			items := make([]outdto.OrderDetailItem, 0, size.details)
			for i := range size.details {
				skuID := snowflake.Next()
				if err := s.db.WithContext(ctx).Create(&basicmodel.SKU{Base: modelbase.Base{ID: skuID}, Code: fmt.Sprintf("SCALE-%d", i), Barcode: fmt.Sprintf("SCALE-BC-%d", i), Name: "scale", Status: 1}).Error; err != nil {
					t.Fatal(err)
				}
				items = append(items, outdto.OrderDetailItem{SKUID: skuID, ExpectedQty: size.batches})
				inventories := make([]*invmodel.Inventory, 0, size.batches)
				locations := make([]*basicmodel.Location, 0, size.batches)
				for j := range size.batches {
					locationID := snowflake.Next()
					locations = append(locations, &basicmodel.Location{Base: modelbase.Base{ID: locationID}, WarehouseID: s.warehouseID, Code: fmt.Sprintf("SCALE-%d-%d", i, j), Status: 1})
					inventories = append(inventories, &invmodel.Inventory{Base: modelbase.Base{ID: snowflake.Next()}, WarehouseID: s.warehouseID, LocationID: locationID, SKUID: skuID, BatchNo: fmt.Sprintf("B%d", j), StockQuantity: 1, AvailableQty: 1, StockInTime: time.Unix(int64(j+1), 0)})
				}
				if err := s.db.WithContext(ctx).CreateInBatches(locations, 100).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.db.WithContext(ctx).CreateInBatches(inventories, 100).Error; err != nil {
					t.Fatal(err)
				}
			}
			samples := 1
			if os.Getenv("WMS_SCALE_PROFILE") == "1" {
				samples = 5
			}
			elapsed := make([]float64, 0, samples)
			for sample := range samples {
				order, err := s.out.Create(ctx, &outdto.CreateOrderReq{WarehouseID: s.warehouseID, BizOrderNo: fmt.Sprintf("SCALE-%d", sample), Details: items}, "scale")
				if err != nil {
					t.Fatal(err)
				}
				if err := s.out.Submit(ctx, order.ID); err != nil {
					t.Fatal(err)
				}
				start := time.Now()
				err = s.out.Approve(ctx, order.ID, "scale")
				elapsed = append(elapsed, float64(time.Since(start).Microseconds())/1000)
				if err != nil {
					t.Fatal(err)
				}
				want := int64(size.details * size.batches)
				for _, model := range []any{&outmodel.Allocation{}, &taskmodel.Task{}} {
					var count int64
					if err := s.db.WithContext(ctx).Model(model).Where("order_id = ?", order.ID).Count(&count).Error; err != nil || count != want {
						t.Fatalf("row count=%d want=%d err=%v", count, want, err)
					}
				}
				var flowCount int64
				if err := s.db.WithContext(ctx).Model(&invmodel.InventoryTrans{}).Where("order_no = ? AND trans_type = ?", order.OrderNo, invmodel.TransAllocate).Count(&flowCount).Error; err != nil || flowCount != want {
					t.Fatalf("allocate flows=%d want=%d err=%v", flowCount, want, err)
				}
				if err := s.out.Cancel(ctx, order.ID, "scale"); err != nil {
					t.Fatal(err)
				}
				var bad int64
				if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).Where("stock_quantity <> available_quantity + allocated_quantity OR available_quantity < 0 OR allocated_quantity <> 0").Count(&bad).Error; err != nil || bad != 0 {
					t.Fatalf("inventory after release: bad=%d err=%v", bad, err)
				}
			}
			sort.Float64s(elapsed)
			t.Logf("SCALE_RESULT details=%d batches_per_detail=%d allocations=%d samples=%d median_ms=%.3f max_ms=%.3f", size.details, size.batches, size.details*size.batches, samples, elapsed[len(elapsed)/2], elapsed[len(elapsed)-1])
			if os.Getenv("WMS_SCALE_PROFILE") == "1" {
				var plan []struct {
					Plan string `gorm:"column:EXPLAIN"`
				}
				query := "EXPLAIN FORMAT=TREE SELECT id, available_quantity, stock_in_time FROM wms_inventory WHERE tenant_id = ? AND warehouse_id = ? AND sku_id = ? AND available_quantity > 0 AND deleted_at IS NULL ORDER BY stock_in_time, id LIMIT 20"
				if err := s.db.WithContext(ctx).Raw(query, 81, s.warehouseID, items[0].SKUID).Scan(&plan).Error; err != nil {
					t.Fatal(err)
				}
				for _, row := range plan {
					t.Logf("SCALE_PLAN %s", row.Plan)
				}
			}
		})
	}
}
