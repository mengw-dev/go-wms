package app

import (
	"context"
	"testing"

	basicdto "gowms/internal/modules/basic/dto"
	basicmodel "gowms/internal/modules/basic/model"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
)

func TestExternalReplayAfterSKUCodeCaseUpdate(t *testing.T) {
	db, _, skus := integrationFixture(t)
	router := integrationRouter(t, db, 11)
	ctx := tenant.WithTenant(context.Background(), 11)
	first := externalResult(t, pushExternal(ctx, router, "CASE-UPDATE", "SHARED-WH"))
	var sku basicmodel.SKU
	if err := db.WithContext(ctx).First(&sku, skus[11]).Error; err != nil {
		t.Fatal(err)
	}
	basic := basicservice.New(basicrepo.New(), tx.New(db), nil, nil, config.LimitsConfig{})
	if err := basic.UpdateSKU(ctx, sku.ID, &basicdto.SKUReq{Code: "shared-sku", Barcode: sku.Barcode, Name: sku.Name, Unit: sku.Unit}); err != nil {
		t.Fatal(err)
	}
	replay := externalResult(t, pushExternal(ctx, router, "CASE-UPDATE", "SHARED-WH"))
	if !replay.Idempotent || replay.OrderID != first.OrderID {
		t.Fatalf("changed casing broke stable identity replay: %+v", replay)
	}
	var detail model.ShipmentOrderDetail
	if err := db.WithContext(ctx).Where("order_id = ?", first.OrderID).First(&detail).Error; err != nil {
		t.Fatal(err)
	}
	if detail.SKUCode != sharedFixtureSKU {
		t.Fatalf("historical code snapshot changed: %s", detail.SKUCode)
	}
}
