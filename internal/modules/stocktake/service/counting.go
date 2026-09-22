package service

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
)

// 实盘数量录入。

func (s *Service) RecordActual(ctx context.Context, orderID, detailID int64, actualQty int) error {
	if actualQty < 0 {
		return errcode.StocktakeQtyInvalid
	}
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.StocktakeNotFound
			}
			return err
		}
		if o.Status != model.OrderDraft {
			return errcode.StocktakeStatusWrong
		}
		d, err := s.repo.GetDetail(tx, detailID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.StocktakeNotFound
			}
			return err
		}
		if d.OrderID != orderID {
			return errcode.StocktakeNotFound
		}
		return s.repo.UpdateDetailActual(tx, detailID, actualQty)
	})
}
