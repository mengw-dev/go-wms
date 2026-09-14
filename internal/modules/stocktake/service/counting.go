package service

import (
	"context"

	"gorm.io/gorm"

	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
)

// 实盘数量录入。

func (s *Service) RecordActual(ctx context.Context, orderID, detailID int64, actualQty int) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			return errcode.StocktakeNotFound
		}
		if o.Status != model.OrderDraft {
			return errcode.StocktakeStatusWrong
		}
		d, err := s.repo.GetDetail(tx, detailID)
		if err != nil || d.OrderID != orderID {
			return errcode.StocktakeNotFound
		}
		return s.repo.UpdateDetailActual(tx, detailID, actualQty)
	})
}
