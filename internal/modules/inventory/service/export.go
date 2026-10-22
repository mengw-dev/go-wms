package service

import (
	"context"
	"encoding/json"
	"strconv"

	"gowms/internal/modules/inventory/dto"
	"gowms/internal/pkg/xlsxexport"
)

// ExportInventory exports all matching stock rows without changing inventory.
func (s *Service) ExportInventory(ctx context.Context, q *dto.InventoryExportQuery) (*xlsxexport.File, error) {
	ctx, release, err := xlsxexport.Start(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	data, err := s.repo.ExportStock(ctx, s.tm.DB(), q, xlsxexport.MaxRows+1)
	if err != nil {
		return nil, err
	}
	if len(data) > xlsxexport.MaxRows {
		return nil, xlsxexport.TooManyRows
	}
	rows := make([][]any, 0, len(data))
	for _, r := range data {
		rows = append(rows, []any{
			strconv.FormatInt(r.ID, 10),
			strconv.FormatInt(r.WarehouseID, 10),
			r.WarehouseCode,
			r.WarehouseName,
			strconv.FormatInt(r.LocationID, 10),
			r.LocationCode,
			strconv.FormatInt(r.SKUID, 10),
			r.SKUCode,
			r.SKUName,
			r.Barcode,
			r.Unit,
			r.BatchNo,
			r.StockQuantity,
			r.AvailableQuantity,
			r.AllocatedQuantity,
			xlsxexport.Time(r.StockInTime),
		})
	}
	filters, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	return xlsxexport.Build(ctx, "inventory", "库存明细", []string{"库存ID", "仓库ID", "仓库编码", "仓库名称", "库位ID", "库位编码", "货品ID", "货品编码", "货品名称", "条码", "单位", "批次", "现存量", "可用量", "分配量", "入库时间（UTC+08:00）"}, rows, string(filters), []string{"已删除或不可见的基础资料留空，但保留库存行及ID。停用资料不隐藏。"})
}

// ExportTransactions exports immutable ledger values with current catalog context.
func (s *Service) ExportTransactions(ctx context.Context, q *dto.TransExportQuery) (*xlsxexport.File, error) {
	ctx, release, err := xlsxexport.Start(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	data, err := s.repo.ExportTransactions(ctx, s.tm.DB(), q, xlsxexport.MaxRows+1)
	if err != nil {
		return nil, err
	}
	if len(data) > xlsxexport.MaxRows {
		return nil, xlsxexport.TooManyRows
	}
	rows := make([][]any, 0, len(data))
	for _, r := range data {
		rows = append(rows, []any{
			strconv.FormatInt(r.ID, 10),
			strconv.FormatInt(r.InventoryID, 10),
			strconv.FormatInt(r.WarehouseID, 10),
			r.WarehouseCode,
			r.LocationCode,
			r.SKUCode,
			r.SKUName,
			r.Unit,
			r.BatchNo,
			r.TransType,
			r.QuantityChange,
			r.BeforeQuantity,
			r.AfterQuantity,
			r.AvailableBefore,
			r.AvailableAfter,
			r.OrderNo,
			r.TaskNo,
			r.Operator,
			xlsxexport.Time(r.CreatedAt),
		})
	}
	filters, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	return xlsxexport.Build(ctx, "inventory-transactions", "库存流水", []string{"流水ID", "库存ID", "仓库ID", "仓库编码", "库位编码", "货品编码", "货品名称", "单位", "批次", "流水类型", "变动数量", "变动前现存量", "变动后现存量", "变动前可用量", "变动后可用量", "来源单据", "任务号", "操作员", "发生时间（UTC+08:00）"}, rows, string(filters), []string{"流水原值不修改；关联库存或基础资料已删除时，辅助字段可能留空。", "分配/释放改变可用量和分配量，不代表现存量同时改变。"})
}
