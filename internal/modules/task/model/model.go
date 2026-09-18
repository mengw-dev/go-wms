package model

import (
	"gowms/internal/modules/system/model"
)

// TaskType 统一任务类型：收货/上架/拣货共用 wms_task。
type TaskType string

const (
	TaskReceive TaskType = "RECEIVE"
	TaskPutaway TaskType = "PUTAWAY"
	TaskPick    TaskType = "PICK"
)

// TaskStatus 任务状态机：CREATED → IN_PROGRESS → COMPLETED（可 CANCELLED）。
type TaskStatus string

const (
	TaskCreated    TaskStatus = "CREATED"
	TaskInProgress TaskStatus = "IN_PROGRESS"
	TaskCompleted  TaskStatus = "COMPLETED"
	TaskCancelled  TaskStatus = "CANCELLED"
)

// StatusTransitions 任务状态转换表。
// CREATED 可一次报满直达 COMPLETED；两个未完成态均可被取消；终态无后继。
var StatusTransitions = map[TaskStatus][]TaskStatus{
	TaskCreated:    {TaskInProgress, TaskCompleted, TaskCancelled},
	TaskInProgress: {TaskCompleted, TaskCancelled},
}

// CanTransit 判断任务状态是否允许从 from 流转到 to。
func CanTransit(from, to TaskStatus) bool {
	for _, next := range StatusTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// Task 统一任务表：任务状态只能单向流转，由 task.Service 校验。
type Task struct {
	model.Base
	model.Versioned
	TenantID    int64      `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_task_no,priority:1;index:idx_task_tenant"`
	TaskNo      string     `json:"task_no" gorm:"size:64;uniqueIndex:uk_task_no,priority:2;not null"`
	TaskType     TaskType   `json:"task_type" gorm:"size:16;index;not null"`
	Status       TaskStatus `json:"status" gorm:"size:16;index;not null;default:'CREATED'"`
	OrderID      int64      `json:"order_id,string" gorm:"index;not null"` // 来源单据 id（入库单/出库单）
	OrderNo      string     `json:"order_no" gorm:"size:64;index"`
	DetailID     int64      `json:"detail_id,string" gorm:"index"`     // 单据明细 id（收货/上架任务）
	AllocationID int64      `json:"allocation_id,string" gorm:"index"` // 拣货任务对应的分配行
	SKUID        int64      `json:"sku_id,string" gorm:"column:sku_id;index;not null"`
	WarehouseID  int64      `json:"warehouse_id,string" gorm:"not null"`
	// 拣货任务的作业位置：库位 + 批次来自分配行，供拣货员直达库位并按批次核对。
	LocationID   int64  `json:"location_id,string" gorm:"index"`
	LocationCode string `json:"location_code" gorm:"size:64"`
	BatchNo      string `json:"batch_no" gorm:"size:64"`
	TargetQty    int    `json:"target_qty" gorm:"not null"`
	DoneQty      int    `json:"done_qty" gorm:"not null;default:0"`
	Operator     string `json:"operator" gorm:"size:64"`
}

func (Task) TableName() string { return "wms_task" }
