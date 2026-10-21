package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
)

func alertFixture(t *testing.T) (*Service, *gorm.DB, context.Context) {
	t.Helper()
	s, _, db := newTestService(t)
	if err := db.AutoMigrate(&model.AlertRule{}); err != nil {
		t.Fatal(err)
	}
	ctx := tenant.WithTenant(context.Background(), 11)
	for _, tid := range []int64{0, 11, 22} {
		c := tenant.WithExactTenant(ctx, tid)
		for _, row := range []any{&basicmodel.Warehouse{Base: modelbase.Base{ID: tid*100 + 1}, Code: "W", Name: "warehouse", Status: 1}, &basicmodel.SKU{Base: modelbase.Base{ID: tid*100 + 1}, Code: "S%_", Barcode: "00123", Name: "sku", Unit: "件", Status: 1}} {
			if err := db.WithContext(c).Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return s, db, ctx
}

func saveRule(ctx context.Context, t *testing.T, s *Service, wh, sku int64, min int, enabled bool) int64 {
	t.Helper()
	r, err := s.SaveAlertRule(ctx, &dto.SaveAlertRuleReq{WarehouseID: wh, SKUID: sku, MinAvailableQty: min, Enabled: &enabled}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func alerts(ctx context.Context, t *testing.T, s *Service, state string) *dto.AlertListResp {
	t.Helper()
	r, err := s.ListAlerts(ctx, &dto.AlertQuery{State: state, Page: 1, PageSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAlertAvailableQuantityAndCurrentRecovery(t *testing.T) {
	s, db, ctx := alertFixture(t)
	id := saveRule(ctx, t, s, 1101, 1101, 10, true)
	r := alerts(ctx, t, s, "ALERT")
	if r.Total != 1 || r.List[0].State != "OUT_OF_STOCK" || r.List[0].GapQty != 10 {
		t.Fatalf("missing stock not warned: %+v", r)
	}
	for i, qty := range []int{3, 2} {
		if err := db.WithContext(ctx).Create(&model.Inventory{Base: modelbase.Base{ID: int64(i + 1)}, StockInTime: time.Now(), WarehouseID: 1101, LocationID: int64(i + 1), SKUID: 1101, BatchNo: fmt.Sprint(i), StockQuantity: qty + 2, AvailableQty: qty, AllocatedQty: 2}).Error; err != nil {
			t.Fatal(err)
		}
	}
	r = alerts(ctx, t, s, "LOW")
	if r.Total != 1 || r.List[0].AvailableQty != 5 || r.List[0].StockQuantity != 9 || r.List[0].AllocatedQty != 4 || r.List[0].GapQty != 5 {
		t.Fatalf("wrong aggregation: %+v", r.List)
	}
	// A different tenant's malformed reference must not inflate the own aggregate.
	if err := db.WithContext(tenant.WithTenant(ctx, 22)).Create(&model.Inventory{Base: modelbase.Base{ID: 99}, StockInTime: time.Now(), WarehouseID: 1101, SKUID: 1101, LocationID: 99, BatchNo: "foreign", StockQuantity: 1000, AvailableQty: 1000}).Error; err != nil {
		t.Fatal(err)
	}
	if got := alerts(ctx, t, s, "LOW").List[0].AvailableQty; got != 5 {
		t.Fatalf("cross tenant sum=%d", got)
	}
	if err := db.WithContext(ctx).Model(&model.Inventory{}).Where("id = 1").Updates(map[string]any{"available_quantity": 8, "stock_quantity": 10}).Error; err != nil {
		t.Fatal(err)
	}
	if r = alerts(ctx, t, s, "ALERT"); r.Total != 0 || r.Summary.Configured != 1 {
		t.Fatalf("exact threshold must recover: %+v", r)
	}
	if id2 := saveRule(ctx, t, s, 1101, 1101, 11, true); id2 != id {
		t.Fatal("PUT duplicated tuple")
	}
	if alerts(ctx, t, s, "ALERT").Total != 1 {
		t.Fatal("updated threshold unused")
	}
	saveRule(ctx, t, s, 1101, 1101, 11, false)
	if alerts(ctx, t, s, "ALERT").Total != 0 || alerts(ctx, t, s, "PAUSED").Total != 1 {
		t.Fatal("pause still warns")
	}
	if err := s.DeleteAlertRule(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAlertRule(ctx, id); err != nil {
		t.Fatal("DELETE replay must succeed", err)
	}
	if alerts(ctx, t, s, "ALL").Total != 0 {
		t.Fatal("deleted rule still present")
	}
	// Configuration removal never alters stock.
	var inv model.Inventory
	if err := db.WithContext(ctx).First(&inv, 1).Error; err != nil || inv.AvailableQty != 8 {
		t.Fatalf("rule wrote stock: %+v %v", inv, err)
	}
	saveRule(ctx, t, s, 1101, 1101, 10, true)
	if alerts(ctx, t, s, "ALL").Total != 1 {
		t.Fatal("cannot recreate tuple")
	}
}

func TestAlertTenantZeroReferencesAndDisabledCatalog(t *testing.T) {
	s, db, ctx := alertFixture(t)
	ids := map[int64]int64{}
	for _, tid := range []int64{0, 11, 22} {
		c := tenant.WithExactTenant(ctx, tid)
		ids[tid] = saveRule(c, t, s, tid*100+1, tid*100+1, 5, true)
		if r := alerts(c, t, s, "ALL"); r.Total != 1 || r.List[0].WarehouseID != tid*100+1 {
			t.Fatalf("tenant %d: %+v", tid, r)
		}
		for _, kind := range []string{"warehouse", "sku"} {
			rows, total, err := s.AlertOptions(c, &dto.AlertOptionQuery{Kind: kind, Page: 1, PageSize: 100})
			if err != nil || total != 1 || len(rows) != 1 || rows[0].ID != tid*100+1 {
				t.Fatalf("tenant option %d %s: %v %d %v", tid, kind, rows, total, err)
			}
		}
	}
	if err := s.DeleteAlertRule(ctx, ids[22]); err != nil {
		t.Fatal(err)
	}
	if alerts(tenant.WithTenant(ctx, 22), t, s, "ALL").Total != 1 {
		t.Fatal("cross tenant delete")
	}
	yes := true
	if _, err := s.SaveAlertRule(ctx, &dto.SaveAlertRuleReq{WarehouseID: 2201, SKUID: 1101, MinAvailableQty: 1, Enabled: &yes}, "tester"); err == nil {
		t.Fatal("foreign warehouse accepted")
	}
	if _, err := s.SaveAlertRule(ctx, &dto.SaveAlertRuleReq{WarehouseID: 1101, SKUID: 2201, MinAvailableQty: 1, Enabled: &yes}, "tester"); err == nil {
		t.Fatal("foreign SKU accepted")
	}
	if err := db.WithContext(ctx).Model(&basicmodel.Warehouse{}).Where("id=1101").Update("status", 0).Error; err != nil {
		t.Fatal(err)
	}
	if alerts(ctx, t, s, "ALERT").Total != 0 || alerts(ctx, t, s, "UNAVAILABLE").Total != 1 {
		t.Fatal("disabled warehouse reports active warning")
	}
	if _, err := s.SaveAlertRule(ctx, &dto.SaveAlertRuleReq{WarehouseID: 1101, SKUID: 1101, MinAvailableQty: 1, Enabled: &yes}, "tester"); err == nil {
		t.Fatal("disabled reference accepted")
	}
	if err := db.WithContext(ctx).Model(&basicmodel.Warehouse{}).Where("id=1101").Update("status", 1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Delete(&basicmodel.SKU{}, 1101).Error; err != nil {
		t.Fatal(err)
	}
	if alerts(ctx, t, s, "UNAVAILABLE").Total != 1 {
		t.Fatal("deleted reference disappeared silently")
	}
}

func TestAlertFilteringLargeAggregateAndConcurrentUpsert(t *testing.T) {
	s, db, ctx := alertFixture(t)
	for i := int64(2); i <= 3; i++ {
		if err := db.WithContext(ctx).Create(&basicmodel.SKU{Base: modelbase.Base{ID: 1100 + i}, Code: fmt.Sprint(i), Barcode: fmt.Sprint(i), Status: 1}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []int64{1101, 1102, 1103} {
		saveRule(ctx, t, s, 1101, id, 10, true)
	}
	for i := int64(1); i <= 2; i++ {
		if err := db.WithContext(ctx).Create(&model.Inventory{Base: modelbase.Base{ID: i}, StockInTime: time.Now(), WarehouseID: 1101, LocationID: i, SKUID: 1101, BatchNo: fmt.Sprint(i), StockQuantity: 2147483647, AvailableQty: 2147483647}).Error; err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.ListAlerts(ctx, &dto.AlertQuery{State: "ALL", SKUKeyword: "%_", WarehouseID: 1101, Page: 1, PageSize: 1})
	if err != nil || r.Total != 1 || r.List[0].AvailableQty != 4294967294 || r.List[0].State != "NORMAL" {
		t.Fatalf("literal/aggregate: %+v %v", r, err)
	}
	r, err = s.ListAlerts(ctx, &dto.AlertQuery{State: "ALL", Page: 2, PageSize: 2})
	if err != nil || r.Total != 3 || len(r.List) != 1 || r.Summary.OutOfStock != 2 {
		t.Fatalf("pagination: %+v %v", r, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup
	results := make(chan error, 2)
	for _, min := range []int{20, 30} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			yes := true
			_, e := s.SaveAlertRule(ctx, &dto.SaveAlertRuleReq{WarehouseID: 1101, SKUID: 1101, MinAvailableQty: min, Enabled: &yes}, "tester")
			results <- e
		}()
	}
	workers.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal(e)
		}
	}
	if alerts(ctx, t, s, "ALL").Total != 3 {
		t.Fatal("concurrent PUT duplicated rule")
	}
}
