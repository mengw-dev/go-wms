package service

import (
	"time"

	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
)

// ActivitySnapshot 是演示账号操作记录接口的稳定响应结构。
// 这里显式列出跨模块字段，避免把内部 Model 直接当作 HTTP 契约。
type ActivitySnapshot struct {
	Operations      []*activityOperationResp      `json:"operations"`
	InboundOrders   []*activityInboundOrderResp   `json:"inbound_orders"`
	OutboundOrders  []*activityOutboundOrderResp  `json:"outbound_orders"`
	StocktakeOrders []*activityStocktakeOrderResp `json:"stocktake_orders"`
	Tasks           []*activityTaskResp           `json:"tasks"`
	InventoryTrans  []*activityInventoryTransResp `json:"inventory_trans"`
}

type activityOperationResp struct {
	ID        int64     `json:"id,string"`
	TenantID  int64     `json:"tenant_id,string"`
	UserID    int64     `json:"user_id,string"`
	Username  string    `json:"username"`
	Path      string    `json:"path"`
	Method    string    `json:"method"`
	Params    string    `json:"params"`
	IP        string    `json:"ip"`
	CostMs    int64     `json:"cost_ms"`
	Status    int       `json:"status"`
	Result    string    `json:"result"`
	CreatedAt time.Time `json:"created_at"`
}

type activityInboundOrderResp struct {
	ID           int64                    `json:"id,string"`
	TenantID     int64                    `json:"tenant_id,string"`
	OrderNo      string                   `json:"order_no"`
	WarehouseID  int64                    `json:"warehouse_id,string"`
	Status       inboundmodel.OrderStatus `json:"status"`
	Source       string                   `json:"source"`
	Remark       string                   `json:"remark"`
	ExpectedQty  int                      `json:"expected_qty"`
	ReceivedQty  int                      `json:"received_qty"`
	DefectiveQty int                      `json:"defective_qty"`
	ImportTaskID *string                  `json:"import_task_id"`
	ImportRow    int                      `json:"import_row"`
	CreatedBy    string                   `json:"created_by"`
	CreatedAt    time.Time                `json:"created_at"`
	UpdatedAt    time.Time                `json:"updated_at"`
}

type activityOutboundOrderResp struct {
	ID           int64                     `json:"id,string"`
	TenantID     int64                     `json:"tenant_id,string"`
	OrderNo      string                    `json:"order_no"`
	BizOrderNo   string                    `json:"biz_order_no"`
	WarehouseID  int64                     `json:"warehouse_id,string"`
	Status       outboundmodel.OrderStatus `json:"status"`
	Remark       string                    `json:"remark"`
	ExpectedQty  int                       `json:"expected_qty"`
	AllocatedQty int                       `json:"allocated_qty"`
	PickedQty    int                       `json:"picked_qty"`
	CreatedBy    string                    `json:"created_by"`
	CreatedAt    time.Time                 `json:"created_at"`
	UpdatedAt    time.Time                 `json:"updated_at"`
}

type activityStocktakeOrderResp struct {
	ID           int64                      `json:"id,string"`
	TenantID     int64                      `json:"tenant_id,string"`
	OrderNo      string                     `json:"order_no"`
	WarehouseID  int64                      `json:"warehouse_id,string"`
	LocationID   int64                      `json:"location_id,string"`
	LocationCode string                     `json:"location_code"`
	Status       stocktakemodel.OrderStatus `json:"status"`
	Remark       string                     `json:"remark"`
	CreatedBy    string                     `json:"created_by"`
	CreatedAt    time.Time                  `json:"created_at"`
	UpdatedAt    time.Time                  `json:"updated_at"`
}

type activityTaskResp struct {
	ID           int64                `json:"id,string"`
	TenantID     int64                `json:"tenant_id,string"`
	TaskNo       string               `json:"task_no"`
	TaskType     taskmodel.TaskType   `json:"task_type"`
	Status       taskmodel.TaskStatus `json:"status"`
	OrderID      int64                `json:"order_id,string"`
	OrderNo      string               `json:"order_no"`
	DetailID     int64                `json:"detail_id,string"`
	AllocationID int64                `json:"allocation_id,string"`
	SKUID        int64                `json:"sku_id,string"`
	WarehouseID  int64                `json:"warehouse_id,string"`
	LocationID   int64                `json:"location_id,string"`
	LocationCode string               `json:"location_code"`
	BatchNo      string               `json:"batch_no"`
	TargetQty    int                  `json:"target_qty"`
	DoneQty      int                  `json:"done_qty"`
	Operator     string               `json:"operator"`
	CreatedAt    time.Time            `json:"created_at"`
	UpdatedAt    time.Time            `json:"updated_at"`
}

type activityInventoryTransResp struct {
	ID              int64              `json:"id,string"`
	TenantID        int64              `json:"tenant_id,string"`
	InventoryID     int64              `json:"inventory_id,string"`
	TransType       invmodel.TransType `json:"trans_type"`
	QuantityChange  int                `json:"quantity_change"`
	BeforeQuantity  int                `json:"before_quantity"`
	AfterQuantity   int                `json:"after_quantity"`
	AvailableBefore int                `json:"available_before"`
	AvailableAfter  int                `json:"available_after"`
	OrderNo         string             `json:"order_no"`
	TaskNo          string             `json:"task_no"`
	Operator        string             `json:"operator"`
	CreatedAt       time.Time          `json:"created_at"`
}

func activitySnapshot(
	operations []*sysmodel.SysOperLog,
	inboundOrders []*inboundmodel.ReceiptOrder,
	outboundOrders []*outboundmodel.ShipmentOrder,
	stocktakeOrders []*stocktakemodel.StocktakeOrder,
	tasks []*taskmodel.Task,
	inventoryTrans []*invmodel.InventoryTrans,
) *ActivitySnapshot {
	return &ActivitySnapshot{
		Operations:      activityOperationResponses(operations),
		InboundOrders:   activityInboundOrderResponses(inboundOrders),
		OutboundOrders:  activityOutboundOrderResponses(outboundOrders),
		StocktakeOrders: activityStocktakeOrderResponses(stocktakeOrders),
		Tasks:           activityTaskResponses(tasks),
		InventoryTrans:  activityInventoryTransResponses(inventoryTrans),
	}
}

func activityOperationResponses(items []*sysmodel.SysOperLog) []*activityOperationResp {
	if items == nil {
		return nil
	}
	resp := make([]*activityOperationResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &activityOperationResp{
			ID:        item.ID,
			TenantID:  item.TenantID,
			UserID:    item.UserID,
			Username:  item.Username,
			Path:      item.Path,
			Method:    item.Method,
			Params:    item.Params,
			IP:        item.IP,
			CostMs:    item.CostMs,
			Status:    item.Status,
			Result:    item.Result,
			CreatedAt: item.CreatedAt,
		})
	}
	return resp
}

func activityInboundOrderResponses(items []*inboundmodel.ReceiptOrder) []*activityInboundOrderResp {
	if items == nil {
		return nil
	}
	resp := make([]*activityInboundOrderResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &activityInboundOrderResp{
			ID:           item.ID,
			TenantID:     item.TenantID,
			OrderNo:      item.OrderNo,
			WarehouseID:  item.WarehouseID,
			Status:       item.Status,
			Source:       item.Source,
			Remark:       item.Remark,
			ExpectedQty:  item.ExpectedQty,
			ReceivedQty:  item.ReceivedQty,
			DefectiveQty: item.DefectiveQty,
			ImportTaskID: item.ImportTaskID,
			ImportRow:    item.ImportRow,
			CreatedBy:    item.CreatedBy,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}
	return resp
}

func activityOutboundOrderResponses(items []*outboundmodel.ShipmentOrder) []*activityOutboundOrderResp {
	if items == nil {
		return nil
	}
	resp := make([]*activityOutboundOrderResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &activityOutboundOrderResp{
			ID:           item.ID,
			TenantID:     item.TenantID,
			OrderNo:      item.OrderNo,
			BizOrderNo:   item.BizOrderNo,
			WarehouseID:  item.WarehouseID,
			Status:       item.Status,
			Remark:       item.Remark,
			ExpectedQty:  item.ExpectedQty,
			AllocatedQty: item.AllocatedQty,
			PickedQty:    item.PickedQty,
			CreatedBy:    item.CreatedBy,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}
	return resp
}

func activityStocktakeOrderResponses(items []*stocktakemodel.StocktakeOrder) []*activityStocktakeOrderResp {
	if items == nil {
		return nil
	}
	resp := make([]*activityStocktakeOrderResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &activityStocktakeOrderResp{
			ID:           item.ID,
			TenantID:     item.TenantID,
			OrderNo:      item.OrderNo,
			WarehouseID:  item.WarehouseID,
			LocationID:   item.LocationID,
			LocationCode: item.LocationCode,
			Status:       item.Status,
			Remark:       item.Remark,
			CreatedBy:    item.CreatedBy,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}
	return resp
}

func activityTaskResponses(items []*taskmodel.Task) []*activityTaskResp {
	if items == nil {
		return nil
	}
	resp := make([]*activityTaskResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &activityTaskResp{
			ID:           item.ID,
			TenantID:     item.TenantID,
			TaskNo:       item.TaskNo,
			TaskType:     item.TaskType,
			Status:       item.Status,
			OrderID:      item.OrderID,
			OrderNo:      item.OrderNo,
			DetailID:     item.DetailID,
			AllocationID: item.AllocationID,
			SKUID:        item.SKUID,
			WarehouseID:  item.WarehouseID,
			LocationID:   item.LocationID,
			LocationCode: item.LocationCode,
			BatchNo:      item.BatchNo,
			TargetQty:    item.TargetQty,
			DoneQty:      item.DoneQty,
			Operator:     item.Operator,
			CreatedAt:    item.CreatedAt,
			UpdatedAt:    item.UpdatedAt,
		})
	}
	return resp
}

func activityInventoryTransResponses(items []*invmodel.InventoryTrans) []*activityInventoryTransResp {
	if items == nil {
		return nil
	}
	resp := make([]*activityInventoryTransResp, 0, len(items))
	for _, item := range items {
		resp = append(resp, &activityInventoryTransResp{
			ID:              item.ID,
			TenantID:        item.TenantID,
			InventoryID:     item.InventoryID,
			TransType:       item.TransType,
			QuantityChange:  item.QuantityChange,
			BeforeQuantity:  item.BeforeQuantity,
			AfterQuantity:   item.AfterQuantity,
			AvailableBefore: item.AvailableBefore,
			AvailableAfter:  item.AvailableAfter,
			OrderNo:         item.OrderNo,
			TaskNo:          item.TaskNo,
			Operator:        item.Operator,
			CreatedAt:       item.CreatedAt,
		})
	}
	return resp
}
