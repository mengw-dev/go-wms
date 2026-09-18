package model

import (
	"gowms/internal/modules/system/model"
)

// OrderStatus 入库单状态机：DRAFT → SUBMITTED → APPROVED → RECEIVING → PUTAWAY → COMPLETED（可 CANCELLED）。
type OrderStatus string

const (
	OrderDraft     OrderStatus = "DRAFT"
	OrderSubmitted OrderStatus = "SUBMITTED"
	OrderApproved  OrderStatus = "APPROVED"
	OrderReceiving OrderStatus = "RECEIVING"
	OrderPutaway   OrderStatus = "PUTAWAY"
	OrderCompleted OrderStatus = "COMPLETED"
	OrderCancelled OrderStatus = "CANCELLED"
)

// 单号前缀与来源标识。
const (
	OrderNoPrefix = "RK" // 入库单号前缀
	SourceManual  = "MANUAL"
	SourceImport  = "IMPORT"
)

// StatusTransitions 状态转换表：只允许单向流转，非法流转返回错误。
// APPROVED 可直达 PUTAWAY（首次收货即收齐）；全部残品无上架作业时收齐直达 COMPLETED；
// 终态 COMPLETED/CANCELLED 无后继。
var StatusTransitions = map[OrderStatus][]OrderStatus{
	OrderDraft:     {OrderSubmitted, OrderCancelled},
	OrderSubmitted: {OrderApproved, OrderCancelled},
	OrderApproved:  {OrderReceiving, OrderPutaway, OrderCompleted, OrderCancelled},
	OrderReceiving: {OrderPutaway, OrderCompleted},
	OrderPutaway:   {OrderCompleted},
}

// CanTransit 判断状态是否允许从 from 流转到 to，是状态机校验的唯一入口。
func CanTransit(from, to OrderStatus) bool {
	for _, next := range StatusTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

type ReceiptOrder struct {
	model.Base
	model.Versioned
	TenantID     int64       `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_receipt_no,priority:1;uniqueIndex:uk_import_row,priority:1;index:idx_ro_tenant"`
	OrderNo      string      `json:"order_no" gorm:"size:64;uniqueIndex:uk_receipt_no,priority:2;not null"`
	WarehouseID  int64       `json:"warehouse_id,string" gorm:"not null"`
	Status       OrderStatus `json:"status" gorm:"size:16;index;not null;default:'DRAFT'"`
	Source       string      `json:"source" gorm:"size:16;default:'MANUAL'"` // MANUAL / IMPORT
	Remark       string      `json:"remark" gorm:"size:255"`
	ExpectedQty  int         `json:"expected_qty" gorm:"not null;default:0"`
	ReceivedQty  int         `json:"received_qty" gorm:"not null;default:0"`
	DefectiveQty int         `json:"defective_qty" gorm:"not null;default:0"`
	// 导入幂等键（导入任务 ID + Excel 行号），补偿扫描重跑已处理行时据此去重；
	// 手工单为 NULL，MySQL 唯一索引对 NULL 不去重，不影响手工建单。
	ImportTaskID *string `json:"import_task_id" gorm:"size:64;uniqueIndex:uk_import_row,priority:2"`
	ImportRow    int     `json:"import_row" gorm:"uniqueIndex:uk_import_row,priority:3"`
	CreatedBy    string  `json:"created_by" gorm:"size:64"`
}

func (ReceiptOrder) TableName() string { return "wms_receipt_order" }

type ReceiptOrderDetail struct {
	model.Base
	TenantID     int64  `json:"tenant_id,string" gorm:"not null;default:0"`
	OrderID      int64  `json:"order_id,string" gorm:"index;not null"`
	SKUID        int64  `json:"sku_id,string" gorm:"column:sku_id;not null"`
	SKUCode      string `json:"sku_code" gorm:"size:64"`
	SKUName      string `json:"sku_name" gorm:"size:128"`
	ExpectedQty  int    `json:"expected_qty" gorm:"not null"`
	ReceivedQty  int    `json:"received_qty" gorm:"not null;default:0"`
	DefectiveQty int    `json:"defective_qty" gorm:"not null;default:0"`
	BatchNo      string `json:"batch_no" gorm:"size:64"` // 收货时录入
}

func (ReceiptOrderDetail) TableName() string { return "wms_receipt_order_detail" }

// ImportTaskStatus Excel 异步导入任务状态机。
type ImportTaskStatus string

const (
	ImportPending    ImportTaskStatus = "PENDING"
	ImportProcessing ImportTaskStatus = "PROCESSING"
	ImportCompleted  ImportTaskStatus = "COMPLETED"
	ImportFailed     ImportTaskStatus = "FAILED"
)

// ImportTask 异步导入任务：CAS 更新防重复执行，悬挂任务由定时补偿扫描重跑。
type ImportTask struct {
	model.Base
	TenantID    int64           `json:"tenant_id,string" gorm:"not null;default:0;uniqueIndex:uk_import_task,priority:1;index:idx_import_tenant"`
	TaskID      string          `json:"task_id" gorm:"size:64;uniqueIndex:uk_import_task,priority:2;not null"`
	Status      ImportTaskStatus `json:"status" gorm:"size:16;index;not null;default:'PENDING'"`
	FileName    string           `json:"file_name" gorm:"size:255"`
	FilePath    string           `json:"file_path" gorm:"size:255"`
	TotalRows   int              `json:"total_rows"`
	SuccessRows int              `json:"success_rows"`
	FailRows    int              `json:"fail_rows"`
	ErrorMsg    string           `json:"error_msg" gorm:"size:1024"`
}

func (ImportTask) TableName() string { return "wms_import_task" }
