package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"testing"

	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
)

func TestApproveLegacyFingerprintReplay(t *testing.T) {
	s, db, ctx, inventory := stocktakeFixture(t)
	order, _ := countedOrder(ctx, t, s, 8)
	const key = "approve-legacy"
	if err := s.Approve(ctx, order.ID, "picker", key); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(strconv.FormatInt(order.ID, 10)))
	if err := db.WithContext(ctx).Model(&idempotency.Record{}).Where("idempotency_key = ?", key).Update("request_hash", fmt.Sprintf("%x", sum)).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Approve(ctx, order.ID, "picker", key); err != nil {
		t.Fatalf("legacy fast replay: %v", err)
	}
	if err, handled := s.reconcileApprove(ctx, 11, key, idempotency.Fingerprint(strconv.FormatInt(order.ID, 10))); err != nil || !handled {
		t.Fatalf("legacy reconcile: handled=%v err=%v", handled, err)
	}
	if err, handled := s.reconcileApprove(ctx, 11, key, idempotency.Fingerprint(strconv.FormatInt(order.ID+1, 10))); !errors.Is(err, errcode.IdempotencyKeyReused) || !handled {
		t.Fatalf("changed reconcile: handled=%v err=%v", handled, err)
	}
	if err := db.WithContext(ctx).First(inventory, inventory.ID).Error; err != nil {
		t.Fatal(err)
	}
	var flows int64
	if err := db.WithContext(ctx).Model(&invmodel.InventoryTrans{}).Count(&flows).Error; err != nil {
		t.Fatal(err)
	}
	if inventory.StockQuantity != 8 || flows != 1 {
		t.Fatalf("legacy replay changed inventory: stock=%d flows=%d", inventory.StockQuantity, flows)
	}
}
