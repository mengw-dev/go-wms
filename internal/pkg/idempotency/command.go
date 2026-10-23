package idempotency

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

// Command commits a mutation and its immutable object receipt together. Replay
// reads stay outside the transaction so RR cannot pin a snapshot before locks.
func Command(ctx context.Context, tm *pkgtx.Manager, scope, key string, hash RequestFingerprint, fn func(*gorm.DB) (int64, error)) (int64, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 64 {
		return 0, errcode.ParamError
	}
	ctx = tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
	tid := tenant.FromContext(ctx)
	replay := func() (int64, error) {
		r, e := Find(tm.DB().WithContext(ctx), tid, scope, key)
		if e != nil || r == nil {
			return 0, e
		}
		if !hash.Matches(r.RequestHash) {
			return 0, errcode.IdempotencyKeyReused
		}
		var out struct {
			ObjectID int64 `json:"object_id,string"`
		}
		if json.Unmarshal([]byte(r.ResultJSON), &out) != nil || out.ObjectID <= 0 {
			return 0, errcode.Internal
		}
		return out.ObjectID, nil
	}
	if id, e := replay(); e != nil || id != 0 {
		return id, e
	}
	var id int64
	err := tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(db *gorm.DB) error {
		var e error
		id, e = fn(db)
		if e != nil {
			return e
		}
		if id <= 0 {
			return errcode.Internal
		}
		raw, e := json.Marshal(struct {
			ObjectID int64 `json:"object_id,string"`
		}{id})
		if e != nil {
			return e
		}
		return Insert(db, &Record{ID: snowflake.Next(), TenantID: tid, Scope: scope, IdempotencyKey: key, RequestHash: hash.String(), ObjectID: id, ResultJSON: string(raw)})
	})
	if err == nil {
		return id, nil
	}
	if ctx.Err() != nil {
		return 0, errcode.Wrap(ctx.Err(), errcode.IdempotencyResultUnknown)
	}
	if old, e := replay(); e != nil {
		if errors.Is(e, errcode.IdempotencyKeyReused) {
			return 0, e
		}
		return 0, errcode.Wrap(e, errcode.IdempotencyResultUnknown)
	} else if old != 0 {
		return old, nil
	}
	return 0, err
}
