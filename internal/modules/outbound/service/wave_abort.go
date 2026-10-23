package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/outbound/dto"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/errcode"
)

func (s *Service) AbortWorkWave(ctx context.Context, id int64, req *dto.AbortWaveRequest, operator, key string) (*dto.WaveAck, error) {
	if req == nil || id <= 0 {
		return nil, errcode.ParamError
	}
	reason, e := closureReason(req.Reason)
	if e != nil {
		return nil, e
	}
	input := struct {
		ID        int64
		Confirmed bool
		Reason    string
	}{id, req.ConfirmReturned, reason}
	ctx = waveContext(ctx)
	return s.waveCommand(ctx, key, "abort", operator, input, func(db *gorm.DB) (int64, error) {
		w, e := s.repo.Wave(db, id, true)
		if e != nil {
			return 0, waveError(e)
		}
		if !waveWorking(w) {
			return 0, errcode.WaveStatusWrong
		}
		// Manager permission authorizes stopping another operator's wave. Physical
		// confirmation is mandatory even for a freshly created wave, to stay explicit.
		if !req.ConfirmReturned {
			return 0, errcode.ExceptionNotAllowed
		}
		orders, _, items, e := s.repo.WaveParts(db, id)
		if e != nil {
			return 0, e
		}
		ids := make([]int64, 0, len(orders))
		active := make(map[int64]bool)
		for _, o := range orders {
			if o.ShippedAt == nil {
				ids = append(ids, o.OrderID)
				active[o.ID] = true
			}
		}
		if len(ids) == 0 {
			return 0, errcode.WaveStatusWrong
		}
		tasks, e := s.taskAPI.ListForWave(ctx, db, ids, maxWaveTasks+1, true)
		if e != nil {
			return 0, e
		}
		expected := make(map[int64]bool)
		for _, item := range items {
			if active[item.WaveOrderID] {
				expected[item.TaskID] = true
			}
		}
		taskIDs := make([]int64, 0, len(tasks))
		for _, t := range tasks {
			if !expected[t.ID] || t.WaveID != id || t.DoneQty != 0 || t.Status != taskmodel.TaskCreated {
				return 0, errcode.WaveOrderInvalid
			}
			taskIDs = append(taskIDs, t.ID)
		}
		if len(taskIDs) == 0 || len(taskIDs) > maxWaveTasks || len(taskIDs) != len(expected) {
			return 0, errcode.WaveOrderInvalid
		}
		if e = s.repo.SetTaskWave(db, taskIDs, id, 0); e != nil {
			return 0, e
		}
		return id, s.repo.AbortWave(db, id, reason, operator)
	})
}
