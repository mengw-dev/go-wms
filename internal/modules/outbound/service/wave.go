package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/idempotency"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	pkgtx "gowms/internal/pkg/tx"
)

const waveScope = "outbound.wave"
const maxWaveOrders = 50
const maxWaveTasks = 1000

func waveContext(ctx context.Context) context.Context {
	return tenant.WithExactTenant(ctx, tenant.FromContext(ctx))
}

func waveError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errcode.WaveNotFound
	}
	return err
}

func (s *Service) ListWaves(ctx context.Context, q *dto.WaveQuery) ([]*model.Wave, int64, error) {
	return s.repo.ListWaves(s.tm.DB().WithContext(waveContext(ctx)), q.WarehouseID, q.Keyword, q.Page, q.PageSize)
}

func (s *Service) WaveCandidates(ctx context.Context, q *dto.WaveCandidateQuery) ([]*dto.OrderResp, error) {
	if q.WarehouseID <= 0 || q.Limit < 1 || q.Limit > maxWaveOrders {
		return nil, errcode.ParamError
	}
	ctx = waveContext(ctx)
	orders, err := s.repo.WaveCandidates(s.tm.DB().WithContext(ctx), tenant.FromContext(ctx), q.WarehouseID, q.Keyword, q.Limit)
	if err != nil {
		return nil, err
	}
	resp := make([]*dto.OrderResp, 0, len(orders))
	for _, order := range orders {
		resp = append(resp, orderResponse(order))
	}
	return resp, nil
}

func (s *Service) GetWave(ctx context.Context, id int64) (*dto.WaveDetailResp, error) {
	ctx = waveContext(ctx)
	var detail *dto.WaveDetailResp
	// One read transaction keeps header, stages and quantities on the same snapshot.
	err := s.tm.Tx(ctx, func(db *gorm.DB) error {
		wave, err := s.repo.Wave(db, id, false)
		if err != nil {
			return waveError(err)
		}
		orders, lines, items, err := s.repo.WaveParts(db, id)
		if err != nil {
			return err
		}
		detail = &dto.WaveDetailResp{Wave: wave, Orders: orders, Lines: lines, Items: items}
		return nil
	})
	return detail, err
}

// waveCommand keeps each wave mutation and its acknowledgement in the same transaction.
// All failures reconcile on a fresh connection, preserving unknown results on read failure.
func (s *Service) waveCommand(ctx context.Context, key, action, operator string, payload any, fn func(*gorm.DB) (int64, error)) (*dto.WaveAck, error) {
	ctx = waveContext(ctx)
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 64 || operator == "" {
		return nil, errcode.ParamError
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	hash := idempotency.Fingerprint(action, operator, string(raw))
	tid := tenant.FromContext(ctx)
	var result *dto.WaveAck
	replay := func(db *gorm.DB) (*dto.WaveAck, error) {
		record, findErr := idempotency.Find(db, tid, waveScope, key)
		if findErr != nil || record == nil {
			return nil, findErr
		}
		if !hash.Matches(record.RequestHash) {
			return nil, errcode.IdempotencyKeyReused
		}
		var ack dto.WaveAck
		if json.Unmarshal([]byte(record.ResultJSON), &ack) != nil || ack.WaveID <= 0 {
			return nil, errcode.Internal
		}
		return &ack, nil
	}
	// Read replay outside the business transaction. Under MySQL RR, a replay
	// lookup inside it would fix a snapshot before waiting for the wave lock,
	// causing later line/item reads to miss a previous operator's commit.
	ack, replayErr := replay(s.tm.DB().WithContext(ctx))
	if replayErr != nil || ack != nil {
		return ack, replayErr
	}
	err = s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(db *gorm.DB) error {
		result = nil
		id, execErr := fn(db)
		if execErr != nil {
			return execErr
		}
		result = &dto.WaveAck{WaveID: id}
		encoded, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return marshalErr
		}
		return idempotency.Insert(db, &idempotency.Record{ID: snowflake.Next(), TenantID: tid, Scope: waveScope, IdempotencyKey: key, RequestHash: hash.String(), ObjectID: id, ResultJSON: string(encoded)})
	})
	if err == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		return nil, errcode.Wrap(ctx.Err(), errcode.IdempotencyResultUnknown)
	}
	ack, replayErr = replay(s.tm.DB().WithContext(ctx))
	if replayErr != nil {
		if errors.Is(replayErr, errcode.IdempotencyKeyReused) {
			return nil, replayErr
		}
		return nil, errcode.Wrap(replayErr, errcode.IdempotencyResultUnknown)
	}
	if ack != nil {
		return ack, nil
	}
	return nil, err
}
