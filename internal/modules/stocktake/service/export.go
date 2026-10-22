package service

import (
	"context"
	"encoding/json"
	"strconv"

	"gowms/internal/modules/stocktake/dto"
	"gowms/internal/pkg/xlsxexport"
)

// ExportDetails separates snapshot variance from completed audit adjustments.
func (s *Service) ExportDetails(ctx context.Context, q *dto.ExportQuery) (*xlsxexport.File, error) {
	ctx, release, err := xlsxexport.Start(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	data, err := s.repo.ExportDetails(ctx, s.tm.DB(), q, xlsxexport.MaxRows+1)
	if err != nil {
		return nil, err
	}
	if len(data) > xlsxexport.MaxRows {
		return nil, xlsxexport.TooManyRows
	}
	rows := make([][]any, 0, len(data))
	for _, r := range data {
		var actual, variance, adjustment any
		if r.ActualQty != nil {
			actual = *r.ActualQty
			variance = int64(*r.ActualQty) - int64(r.BookQty)
		}
		if r.Status == "COMPLETED" && r.ActualQty != nil {
			adjustment = r.DiffQty
		}
		adjusted := "否"
		if r.Adjusted {
			adjusted = "是"
		}
		rows = append(rows, []any{
			strconv.FormatInt(r.OrderID, 10),
			r.OrderNo,
			strconv.FormatInt(r.WarehouseID, 10),
			r.Status,
			strconv.FormatInt(r.DetailID, 10),
			strconv.FormatInt(r.InventoryID, 10),
			r.SKUCode,
			r.SKUName,
			r.LocationCode,
			r.BatchNo,
			r.BookQty,
			actual,
			variance,
			adjustment,
			adjusted,
			xlsxexport.Time(r.CreatedAt),
		})
	}
	filters, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	return xlsxexport.Build(ctx, "stocktake-details", "盘点明细与差异", []string{"盘点单ID", "盘点单号", "仓库ID", "单据状态", "明细ID", "库存ID", "货品编码", "货品名称", "库位", "批次", "快照账面数量", "已保存实盘数量", "实盘较快照差异", "审核调整量", "差异已调整", "创建时间（UTC+08:00）"}, rows, string(filters), []string{"未录入实盘留空，已录入0保留数字0；仅导出服务端已保存值。", "实盘较快照差异=实盘-创建时账面；审核调整量为审核时针对当时库存保存的调整量，两者可能不同。未审核或取消的审核调整量留空。", "仅差异行包括实盘与快照不等，或已完成单据的审核调整量非零；未盘行不列入差异。", "盘点单号/货品/库位/批次使用盘点记录中的原值；导出不审核或调整库存。"})
}
