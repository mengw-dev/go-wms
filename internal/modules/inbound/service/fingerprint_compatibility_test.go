package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"gowms/internal/modules/inbound/dto"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
)

func legacyRequestHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return fmt.Sprintf("%x", sum)
}

func TestReceiveLegacyFingerprintReplay(t *testing.T) {
	st := newReceiveIdemStack(t, 10)
	req := &dto.ReceiveReq{DetailID: st.detail.ID, Qty: 10, BatchNo: "B1"}
	const key = "receive-legacy"
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, req, "picker", key); err != nil {
		t.Fatal(err)
	}
	var record idempotency.Record
	db := st.db.WithContext(st.ctx)
	if err := db.Where("idempotency_key = ?", key).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(record.RequestHash, "v2:") {
		t.Fatalf("new record uses old encoding: %s", record.RequestHash)
	}
	legacy := legacyRequestHash(strconv.FormatInt(st.order.ID, 10), strconv.FormatInt(st.detail.ID, 10), "10", "0", "B1")
	if err := db.Model(&record).Update("request_hash", legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, req, "picker", key); err != nil {
		t.Fatalf("legacy fast replay: %v", err)
	}
	if err, handled := st.s.reconcileReceive(st.ctx, 21, key, receiveRequestHash(st.order.ID, st.detail.ID, req)); err != nil || !handled {
		t.Fatalf("legacy reconcile: handled=%v err=%v", handled, err)
	}
	changed := *req
	changed.Qty = 9
	if err, handled := st.s.reconcileReceive(st.ctx, 21, key, receiveRequestHash(st.order.ID, st.detail.ID, &changed)); !errors.Is(err, errcode.IdempotencyKeyReused) || !handled {
		t.Fatalf("changed reconcile: handled=%v err=%v", handled, err)
	}
	order, detail := st.reload(t)
	if order.ReceivedQty != 10 || detail.ReceivedQty != 10 || len(st.putawayTasks(t)) != 1 {
		t.Fatal("legacy replay changed business state")
	}
}

func TestPutawayLegacyFingerprintReplay(t *testing.T) {
	st := newPutawayIdemStack(t)
	const key = "putaway-legacy"
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 10, "picker", key); err != nil {
		t.Fatal(err)
	}
	legacy := legacyRequestHash(strconv.FormatInt(st.task.ID, 10), strconv.FormatInt(st.location, 10), "10")
	if err := st.db.WithContext(st.ctx).Model(&idempotency.Record{}).Where("idempotency_key = ?", key).Update("request_hash", legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.s.Putaway(st.ctx, st.task.ID, st.location, 10, "picker", key); err != nil {
		t.Fatalf("legacy fast replay: %v", err)
	}
	if err, handled := st.s.reconcilePutaway(st.ctx, 31, key, putawayRequestHash(st.task.ID, st.location, 10)); err != nil || !handled {
		t.Fatalf("legacy reconcile: handled=%v err=%v", handled, err)
	}
	if err, handled := st.s.reconcilePutaway(st.ctx, 31, key, putawayRequestHash(st.task.ID, st.location, 9)); !errors.Is(err, errcode.IdempotencyKeyReused) || !handled {
		t.Fatalf("changed reconcile: handled=%v err=%v", handled, err)
	}
	if st.inventory(t).StockQuantity != 10 || st.transCount(t) != 1 || st.reloadTask(t).DoneQty != 10 {
		t.Fatal("legacy replay changed inventory, flows or task")
	}
}

func TestReceiveSeparatorFingerprintReplay(t *testing.T) {
	st := newReceiveIdemStack(t, 10)
	req := &dto.ReceiveReq{DetailID: st.detail.ID, Qty: 10, BatchNo: "A\x1fB"}
	const key = "receive-separator"
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, req, "picker", key); err != nil {
		t.Fatal(err)
	}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, req, "picker", key); err != nil {
		t.Fatalf("v2 separator replay: %v", err)
	}
	if err, handled := st.s.reconcileReceive(st.ctx, 21, key, receiveRequestHash(st.order.ID, st.detail.ID, req)); err != nil || !handled {
		t.Fatalf("v2 separator reconcile: handled=%v err=%v", handled, err)
	}
	// Simulate a pre-upgrade record whose field boundaries cannot be proven generically.
	legacy := legacyRequestHash(strconv.FormatInt(st.order.ID, 10), strconv.FormatInt(st.detail.ID, 10), "10", "0", req.BatchNo)
	if err := st.db.WithContext(st.ctx).Model(&idempotency.Record{}).Where("idempotency_key = ?", key).Update("request_hash", legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := st.s.Receive(st.ctx, st.order.ID, st.detail.ID, req, "picker", key); !errors.Is(err, errcode.IdempotencyKeyReused) {
		t.Fatalf("ambiguous legacy should block: %v", err)
	}
	order, _ := st.reload(t)
	if order.ReceivedQty != 10 || len(st.putawayTasks(t)) != 1 {
		t.Fatal("blocked replay changed business state")
	}
}
