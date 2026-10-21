package app

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"

	outservice "gowms/internal/modules/outbound/service"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
)

func TestOutboundLegacyFingerprintReplay(t *testing.T) {
	for _, scope := range []string{"outbound.pick", "outbound.claim"} {
		t.Run(scope, func(t *testing.T) {
			stack := newPickRaceStack(t)
			_, task := stack.newPickOrder(t, "LEGACY-FP", 1)
			const key = "outbound-legacy"
			var token string
			if scope == "outbound.pick" {
				if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", strictScan(task), "", key); err != nil {
					t.Fatal(err)
				}
			} else {
				result, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "picker", key)
				if err != nil {
					t.Fatal(err)
				}
				token = result.ClaimToken
			}
			parts := []string{strconv.FormatInt(task.ID, 10), "picker"}
			if scope == "outbound.pick" {
				parts = []string{strconv.FormatInt(task.ID, 10), "1", strings.ToLower(task.LocationCode), strings.ToLower(task.BatchNo), "pda"}
			}
			sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
			var record idempotency.Record
			db := stack.db.WithContext(stack.ctx)
			if err := db.Where("scope = ? AND idempotency_key = ?", scope, key).First(&record).Error; err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(record.RequestHash, "v2:") {
				t.Fatalf("new record must use v2: %s", record.RequestHash)
			}
			snapshot := record.ResultJSON
			if err := db.Model(&record).Update("request_hash", fmt.Sprintf("%x", sum)).Error; err != nil {
				t.Fatal(err)
			}
			replay := func() {
				t.Helper()
				if scope == "outbound.pick" {
					result, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", strictScan(task), "expired-token", key)
					if err != nil || result == nil || result.DoneQty != 1 {
						t.Fatalf("legacy pick replay: result=%+v err=%v", result, err)
					}
				} else {
					result, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "picker", key)
					if err != nil || result == nil || result.ClaimToken != token {
						t.Fatalf("legacy claim replay: result=%+v err=%v", result, err)
					}
				}
			}
			replay()
			// Hide only the first lookup: the fallback must still accept the legacy hash
			// after a rolled-back business attempt or transaction retry.
			reads := 0
			const callback = "fingerprint:legacy_miss"
			if err := stack.db.Callback().Query().After("gorm:query").Register(callback, func(q *gorm.DB) {
				if q.Statement.Table == "wms_idempotency" {
					reads++
					if reads == 1 {
						q.Error = gorm.ErrRecordNotFound
					}
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer stack.db.Callback().Query().Remove(callback)
			replay()
			if reads < 2 {
				t.Fatalf("fallback was not exercised: reads=%d", reads)
			}
			if scope == "outbound.pick" {
				if _, err := stack.out.Pick(stack.ctx, task.ID, 2, "picker", strictScan(task), "", key); !errors.Is(err, errcode.IdempotencyKeyReused) {
					t.Fatalf("legacy changed quantity: %v", err)
				}
			} else {
				if _, err := stack.out.ClaimPickTask(stack.ctx, task.ID, "other-picker", key); !errors.Is(err, errcode.IdempotencyKeyReused) {
					t.Fatalf("legacy changed operator: %v", err)
				}
			}
			var after idempotency.Record
			if err := db.First(&after, record.ID).Error; err != nil {
				t.Fatal(err)
			}
			if after.ResultJSON != snapshot || after.RequestHash != record.RequestHash {
				t.Fatal("legacy record must not be rewritten")
			}
			got := stack.reloadTask(t, task.ID)
			if scope == "outbound.pick" && (got.DoneQty != 1 || stack.reloadAllocation(t, task.AllocationID).PickedQty != 1) {
				t.Fatal("legacy replay duplicated pick")
			}
			if scope == "outbound.claim" && got.ClaimToken != token {
				t.Fatal("legacy replay rotated claim token")
			}
		})
	}
}

func TestPickSeparatorFingerprintRejectsShiftedFields(t *testing.T) {
	stack := newPickRaceStack(t)
	_, task := stack.newPickOrder(t, "SEPARATOR-FP", 1)
	first := []string{strconv.FormatInt(task.ID, 10), "1", "a\x1fb", "c", "pda"}
	shifted := &outservice.PickScan{Strict: true, LocationCode: "a", BatchNo: "b\x1fc"}
	// These scans share a legacy byte stream. The fixture is a saved response,
	// not evidence that these scans pass current location/batch business validation.
	sum := sha256.Sum256([]byte(strings.Join(first, "\x1f")))
	for _, stored := range []string{idempotency.Fingerprint(first...).String(), fmt.Sprintf("%x", sum)} {
		if err := stack.db.WithContext(stack.ctx).Create(&idempotency.Record{Scope: "outbound.pick", IdempotencyKey: "separator-key", RequestHash: stored, ObjectID: task.ID, ResultJSON: `{"task_status":"COMPLETED","done_qty":1,"remaining_qty":0,"order_status":"PICKING"}`}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := stack.out.Pick(stack.ctx, task.ID, 1, "picker", shifted, "", "separator-key"); !errors.Is(err, errcode.IdempotencyKeyReused) {
			t.Fatalf("shifted scan accepted: %v", err)
		}
		if got := stack.reloadTask(t, task.ID); got.DoneQty != 0 {
			t.Fatal("rejected scan changed task")
		}
		if err := stack.db.WithContext(stack.ctx).Where("idempotency_key = ?", "separator-key").Delete(&idempotency.Record{}).Error; err != nil {
			t.Fatal(err)
		}
	}
}
