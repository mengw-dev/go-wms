package service

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/errcode"
)

func waveWorking(w *model.Wave) bool {
	return w.Status == model.WaveCreated || w.Status == model.WavePicking || w.Status == model.WaveSorting || w.Status == model.WaveReady
}

func waveOwner(w *model.Wave, operator string) error {
	if w.ClaimedBy != operator || w.LeaseExpireAt == nil || !w.LeaseExpireAt.After(time.Now()) {
		return errcode.WaveClaimRequired
	}
	return nil
}

func normalizeWaveQuantity(req dto.WaveQuantityReq) dto.WaveQuantityReq {
	req.SKUCode = strings.ToLower(strings.TrimSpace(req.SKUCode))
	req.LocationCode = strings.ToLower(strings.TrimSpace(req.LocationCode))
	req.BatchNo = strings.ToLower(strings.TrimSpace(req.BatchNo))
	req.SlotCode = strings.ToLower(strings.TrimSpace(req.SlotCode))
	return req
}

func waveSKUScan(line *model.WaveLine, scan string) bool {
	return scan != "" && (strings.EqualFold(line.SKUCode, scan) || (line.Barcode != "" && strings.EqualFold(line.Barcode, scan)))
}

func (s *Service) ClaimWave(ctx context.Context, id int64, operator, key string) (*dto.WaveAck, error) {
	return s.waveCommand(ctx, key, "claim", operator, id, func(db *gorm.DB) (int64, error) {
		w, err := s.repo.Wave(db, id, true)
		if err != nil {
			return 0, waveError(err)
		}
		if !waveWorking(w) {
			return 0, errcode.WaveStatusWrong
		}
		if w.ClaimedBy != "" && w.ClaimedBy != operator && w.LeaseExpireAt != nil && w.LeaseExpireAt.After(time.Now()) {
			return 0, errcode.WaveClaimRequired
		}
		err = s.repo.UpdateWave(db, id, map[string]any{"claimed_by": operator, "lease_expire_at": time.Now().Add(pickLeaseTTL)})
		return id, err
	})
}

func (s *Service) CollectWave(ctx context.Context, id, lineID int64, req dto.WaveQuantityReq, operator, key string) (*dto.WaveAck, error) {
	req = normalizeWaveQuantity(req)
	payload := struct {
		WaveID, LineID int64
		Req            dto.WaveQuantityReq
	}{id, lineID, req}
	return s.waveCommand(ctx, key, "collect", operator, payload, func(db *gorm.DB) (int64, error) {
		w, err := s.repo.Wave(db, id, true)
		if err != nil {
			return 0, waveError(err)
		}
		if w.Status != model.WaveCreated && w.Status != model.WavePicking {
			return 0, errcode.WaveStatusWrong
		}
		if err = waveOwner(w, operator); err != nil {
			return 0, err
		}
		line, err := s.repo.WaveLine(db, id, lineID)
		if err != nil {
			return 0, waveError(err)
		}
		if req.Qty <= 0 || int64(req.Qty) > line.TargetQty-line.PickedQty {
			return 0, errcode.WaveQuantityWrong
		}
		if !waveSKUScan(line, req.SKUCode) || req.LocationCode == "" || !strings.EqualFold(line.LocationCode, req.LocationCode) || !strings.EqualFold(line.BatchNo, req.BatchNo) {
			return 0, errcode.WaveScanMismatch
		}
		if err = s.repo.IncrementWavePicked(db, lineID, req.Qty); err != nil {
			return 0, err
		}
		remaining, err := s.repo.WaveUnpicked(db, id)
		if err != nil {
			return 0, err
		}
		status := model.WavePicking
		if remaining == 0 {
			status = model.WaveSorting
		}
		err = s.repo.UpdateWave(db, id, map[string]any{"status": status, "lease_expire_at": time.Now().Add(pickLeaseTTL)})
		return id, err
	})
}

func (s *Service) SortWave(ctx context.Context, id, itemID int64, req dto.WaveQuantityReq, operator, key string) (*dto.WaveAck, error) {
	req = normalizeWaveQuantity(req)
	payload := struct {
		WaveID, ItemID int64
		Req            dto.WaveQuantityReq
	}{id, itemID, req}
	return s.waveCommand(ctx, key, "sort", operator, payload, func(db *gorm.DB) (int64, error) {
		w, err := s.repo.Wave(db, id, true)
		if err != nil {
			return 0, waveError(err)
		}
		if w.Status != model.WaveSorting {
			return 0, errcode.WaveStatusWrong
		}
		if err = waveOwner(w, operator); err != nil {
			return 0, err
		}
		item, err := s.repo.WaveItem(db, id, itemID)
		if err != nil {
			return 0, waveError(err)
		}
		member, err := s.repo.WaveOrder(db, id, item.WaveOrderID)
		if err != nil {
			return 0, waveError(err)
		}
		line, err := s.repo.WaveLine(db, id, item.LineID)
		if err != nil {
			return 0, waveError(err)
		}
		if req.Qty <= 0 || req.Qty > item.TargetQty-item.SortedQty {
			return 0, errcode.WaveQuantityWrong
		}
		if !waveSKUScan(line, req.SKUCode) || req.SlotCode == "" || !strings.EqualFold(member.SlotCode, req.SlotCode) || !strings.EqualFold(line.BatchNo, req.BatchNo) {
			return 0, errcode.WaveScanMismatch
		}
		if err = s.repo.IncrementWaveSorted(db, itemID, req.Qty); err != nil {
			return 0, err
		}
		remaining, err := s.repo.WaveUnsorted(db, id)
		if err != nil {
			return 0, err
		}
		status := model.WaveSorting
		if remaining == 0 {
			status = model.WaveReady
		}
		err = s.repo.UpdateWave(db, id, map[string]any{"status": status, "lease_expire_at": time.Now().Add(pickLeaseTTL)})
		return id, err
	})
}

func (s *Service) CancelWave(ctx context.Context, id int64, operator, key string) (*dto.WaveAck, error) {
	ctx = waveContext(ctx)
	return s.waveCommand(ctx, key, "cancel", operator, id, func(db *gorm.DB) (int64, error) {
		w, err := s.repo.Wave(db, id, true)
		if err != nil {
			return 0, waveError(err)
		}
		if w.Status == model.WaveCancelled {
			return id, nil
		}
		if w.Status != model.WaveCreated {
			return 0, errcode.WaveStatusWrong
		}
		orders, lines, _, err := s.repo.WaveParts(db, id)
		if err != nil {
			return 0, err
		}
		for _, line := range lines {
			if line.PickedQty != 0 {
				return 0, errcode.WaveStatusWrong
			}
		}
		ids := make([]int64, 0, len(orders))
		for _, o := range orders {
			ids = append(ids, o.OrderID)
		}
		tasks, err := s.taskAPI.ListForWave(ctx, db, ids, maxWaveTasks+1, true)
		if err != nil {
			return 0, err
		}
		taskIDs := make([]int64, 0, len(tasks))
		for _, task := range tasks {
			if task.WaveID != id {
				return 0, errcode.WaveOrderInvalid
			}
			taskIDs = append(taskIDs, task.ID)
		}
		if len(taskIDs) == 0 || len(taskIDs) > maxWaveTasks {
			return 0, errcode.WaveOrderInvalid
		}
		if err = s.repo.SetTaskWave(db, taskIDs, id, 0); err != nil {
			return 0, err
		}
		if err = s.repo.ReleaseWaveOrders(db, id); err != nil {
			return 0, err
		}
		err = s.repo.UpdateWave(db, id, map[string]any{"status": model.WaveCancelled, "claimed_by": "", "lease_expire_at": nil})
		return id, err
	})
}
