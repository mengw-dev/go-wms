package service

import (
	"context"
	"errors"
	"sort"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
	pkgtx "gowms/internal/pkg/tx"
)

// 盘点审核和库存调整。

func (s *Service) Approve(ctx context.Context, orderID int64, operator string) error {
	return s.tm.TxRetry(ctx, pkgtx.MaxTxRetry, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errcode.StocktakeNotFound
			}
			return err
		}
		if !model.CanTransit(o.Status, model.OrderCompleted) {
			return errcode.StocktakeStatusWrong
		}
		details, err := s.repo.ListDetails(tx, orderID)
		if err != nil {
			return err
		}
		// 盘点之间按库存 ID 统一加锁顺序；与其他业务仍可能竞争，死锁由整事务重试处理。
		sort.Slice(details, func(i, j int) bool { return details[i].InventoryID < details[j].InventoryID })
		anyCounted := false
		for _, d := range details {
			if d.ActualQty == nil || d.Adjusted {
				continue
			}
			anyCounted = true
			// 差异和流水使用同一份加锁后的库存；零差异也校验库存是否存在。
			diff, err := s.inv.Adjust(ctx, tx, &api.AdjustReq{
				InventoryID: d.InventoryID, NewStock: *d.ActualQty,
				OrderNo: o.OrderNo, Operator: operator,
			})
			if err != nil {
				return err
			}
			if err := s.repo.MarkAdjusted(tx, d.ID, diff); err != nil {
				return err
			}
		}
		if !anyCounted {
			return errcode.StocktakeNoDetail
		}
		if n, err := s.repo.UpdateStatus(tx, orderID, o.Status, model.OrderCompleted); err != nil {
			return err
		} else if n == 0 {
			return errcode.StocktakeVersionBad
		}
		return nil
	})
}
