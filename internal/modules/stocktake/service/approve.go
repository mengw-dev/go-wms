package service

import (
	"context"
	"sort"

	"gorm.io/gorm"

	"gowms/internal/modules/inventory/api"
	"gowms/internal/modules/stocktake/model"
	"gowms/internal/pkg/errcode"
)

// 盘点审核和库存调整。

func (s *Service) Approve(ctx context.Context, orderID int64, operator string) error {
	return s.tm.Tx(ctx, func(tx *gorm.DB) error {
		o, err := s.repo.GetOrderForUpdate(tx, orderID)
		if err != nil {
			return errcode.StocktakeNotFound
		}
		if o.Status != model.OrderDraft {
			return errcode.StocktakeStatusWrong
		}
		details, err := s.repo.ListDetails(tx, orderID)
		if err != nil {
			return err
		}
		// 按库存行 ID 排序后再调整：统一加锁顺序，
		// 避免与出库审核等并发事务交叉加锁导致死锁
		sort.Slice(details, func(i, j int) bool { return details[i].InventoryID < details[j].InventoryID })
		anyCounted := false
		for _, d := range details {
			if d.ActualQty == nil || d.Adjusted {
				continue
			}
			anyCounted = true
			actual := *d.ActualQty
			// 以当前实时库存重算差异（快照后库存可能已变动）
			var current int
			if err := tx.Table("wms_inventory").Where("id = ?", d.InventoryID).Pluck("stock_quantity", &current).Error; err != nil {
				continue // 库存行已删除，跳过
			}
			diff := actual - current
			if diff != 0 {
				if err := s.inv.Adjust(ctx, tx, &api.AdjustReq{
					InventoryID: d.InventoryID, NewStock: actual,
					OrderNo: o.OrderNo, Operator: operator,
				}); err != nil {
					return err
				}
			}
			if err := s.repo.MarkAdjusted(tx, d.ID, diff); err != nil {
				return err
			}
		}
		if !anyCounted {
			return errcode.StocktakeNoDetail
		}
		if n, err := s.repo.UpdateStatus(tx, orderID, model.OrderDraft, model.OrderCompleted); err != nil {
			return err
		} else if n == 0 {
			return errcode.StocktakeVersionBad
		}
		return nil
	})
}
