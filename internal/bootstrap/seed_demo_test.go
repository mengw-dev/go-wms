package bootstrap

import (
	"context"
	"strings"
	"testing"

	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/tenant"
)

func TestDemoSeedIsOpeningInventoryWithoutFabricatedBusinessChains(t *testing.T) {
	db := resetTestDB(t)
	ctx := tenant.WithTenant(context.Background(), 100)
	if err := seedDemoData(db.WithContext(ctx)); err != nil {
		t.Fatal(err)
	}
	for _, entity := range []any{&inboundmodel.ReceiptOrder{}, &inboundmodel.ReceiptOrderDetail{}, &outboundmodel.ShipmentOrder{}, &outboundmodel.ShipmentOrderDetail{}, &outboundmodel.Allocation{}, &taskmodel.Task{}} {
		var count int64
		if err := db.WithContext(ctx).Model(entity).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("seed fabricated %T rows: %d", entity, count)
		}
	}
	var inventory []invmodel.Inventory
	var transfers []invmodel.InventoryTrans
	if err := db.WithContext(ctx).Find(&inventory).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Find(&transfers).Error; err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 4 || len(transfers) != 4 {
		t.Fatalf("inventory/transfers=%d/%d", len(inventory), len(transfers))
	}
	total := 0
	for _, item := range inventory {
		matches := 0
		for _, transfer := range transfers {
			if transfer.InventoryID != item.ID {
				continue
			}
			matches++
			if transfer.TransType != invmodel.TransOpening || transfer.Operator != "system-seed" || !strings.HasPrefix(transfer.OrderNo, "SEED-") ||
				transfer.BeforeQuantity != 0 || transfer.AvailableBefore != 0 || transfer.QuantityChange != item.StockQuantity ||
				transfer.AfterQuantity != item.StockQuantity || transfer.AvailableAfter != item.AvailableQty || item.AllocatedQty != 0 {
				t.Fatalf("opening balance mismatch: inventory=%+v transfer=%+v", item, transfer)
			}
		}
		if matches != 1 {
			t.Fatalf("inventory %d opening records=%d", item.ID, matches)
		}
		total += item.StockQuantity
	}
	if total != 230 {
		t.Fatalf("opening quantity=%d want 230", total)
	}
	before := tenantCountsOf(ctx, t, db)
	if err := seedDemoData(db.WithContext(ctx)); err != nil {
		t.Fatal(err)
	}
	if after := tenantCountsOf(ctx, t, db); before != after {
		t.Fatalf("repeated seed changed data: %+v -> %+v", before, after)
	}
}

func TestDemoSeedPreservesExistingBusinessData(t *testing.T) {
	db := resetTestDB(t)
	ctx := tenant.WithTenant(context.Background(), 100)
	seedTenantSentinel(ctx, t, db, "EXISTING")
	before := tenantCountsOf(ctx, t, db)
	if err := seedDemoData(db.WithContext(ctx)); err != nil {
		t.Fatal(err)
	}
	if after := tenantCountsOf(ctx, t, db); before != after {
		t.Fatalf("seed modified existing data: %+v -> %+v", before, after)
	}
}
