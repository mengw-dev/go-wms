// Package dto 定义任务模块的 HTTP 响应结构。
package dto

import (
	"time"

	"gowms/internal/modules/task/model"
)

// TaskResp 是任务查询接口的稳定响应契约，不直接暴露 GORM Model。
type TaskResp struct {
	ID           int64            `json:"id,string"`
	TenantID     int64            `json:"tenant_id,string"`
	TaskNo       string           `json:"task_no"`
	TaskType     model.TaskType   `json:"task_type"`
	Status       model.TaskStatus `json:"status"`
	OrderID      int64            `json:"order_id,string"`
	OrderNo      string           `json:"order_no"`
	DetailID     int64            `json:"detail_id,string"`
	AllocationID int64            `json:"allocation_id,string"`
	SKUID        int64            `json:"sku_id,string"`
	WarehouseID  int64            `json:"warehouse_id,string"`
	LocationID   int64            `json:"location_id,string"`
	LocationCode string           `json:"location_code"`
	BatchNo      string           `json:"batch_no"`
	TargetQty    int              `json:"target_qty"`
	DoneQty      int              `json:"done_qty"`
	Operator     string           `json:"operator"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}
