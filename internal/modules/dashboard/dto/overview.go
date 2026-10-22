// Package dto defines the read-only warehouse dashboard contract.
package dto

import "time"

type Access struct {
	Inbound   bool `json:"inbound"`
	Outbound  bool `json:"outbound"`
	Tasks     bool `json:"tasks"`
	Inventory bool `json:"inventory"`
	Basic     bool `json:"basic"`
	Stocktake bool `json:"stocktake"`
}

type StatusCount struct {
	Status string `json:"status"`
	Total  int64  `json:"total"`
	Today  int64  `json:"today"`
}

type DayCount struct {
	Date  string `json:"date"`
	Total int64  `json:"total"`
}

type Order struct {
	ID          int64     `json:"id,string"`
	OrderNo     string    `json:"order_no"`
	WarehouseID int64     `json:"warehouse_id,string"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type Orders struct {
	Statuses []StatusCount `json:"statuses"`
	Days     []DayCount    `json:"days"`
	Recent   []Order       `json:"recent"`
}

type TaskCount struct {
	TaskType string `json:"task_type"`
	Status   string `json:"status"`
	Total    int64  `json:"total"`
	Older24h int64  `json:"older_24h" gorm:"column:older_24h"`
}

type Task struct {
	ID          int64     `json:"id,string"`
	OrderID     int64     `json:"order_id,string"`
	OrderNo     string    `json:"order_no"`
	TaskNo      string    `json:"task_no"`
	TaskType    string    `json:"task_type"`
	Status      string    `json:"status"`
	WarehouseID int64     `json:"warehouse_id,string"`
	CreatedAt   time.Time `json:"created_at"`
}

type Inventory struct {
	SKUs          int64 `json:"skus" gorm:"column:skus"`
	Rows          int64 `json:"rows"`
	AllocatedRows int64 `json:"allocated_rows"`
}

type Locations struct {
	Enabled  int64 `json:"enabled"`
	Occupied int64 `json:"occupied"`
}

type Warehouse struct {
	ID   int64  `json:"id,string"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Activity counts immutable stock ledger records, never cross-SKU quantities or
// task completions inferred from mutable updated_at timestamps.
type Activity struct {
	PutawayRecords  int64            `json:"putaway_records"`
	ShipmentRecords int64            `json:"shipment_records"`
	Recent          []ActivityRecord `json:"recent" gorm:"-"`
}

type ActivityRecord struct {
	ID          int64     `json:"id,string"`
	InventoryID int64     `json:"inventory_id,string"`
	TransType   string    `json:"trans_type"`
	OrderNo     string    `json:"order_no"`
	TaskNo      string    `json:"task_no"`
	CreatedAt   time.Time `json:"created_at"`
}

type ImportIssue struct {
	TaskID      string    `json:"task_id"`
	FileName    string    `json:"file_name"`
	Status      string    `json:"status"`
	SuccessRows int       `json:"success_rows"`
	FailRows    int       `json:"fail_rows"`
	ErrorMsg    string    `json:"error_msg"`
	CreatedAt   time.Time `json:"created_at"`
}

// Imports spans warehouses: a single file can contain several warehouses.
type Imports struct {
	Total  int64         `json:"total"`
	Recent []ImportIssue `json:"recent"`
}

type Overview struct {
	GeneratedAt     time.Time   `json:"generated_at"`
	TodayFrom       string      `json:"today_from"`
	TodayTo         string      `json:"today_to"`
	TrendFrom       string      `json:"trend_from"`
	WarehouseID     int64       `json:"warehouse_id,string"`
	Access          Access      `json:"access"`
	Warehouses      []Warehouse `json:"warehouses"`
	Inbound         *Orders     `json:"inbound,omitempty"`
	Outbound        *Orders     `json:"outbound,omitempty"`
	TaskCounts      []TaskCount `json:"task_counts,omitempty"`
	OldestTasks     []Task      `json:"oldest_tasks,omitempty"`
	Inventory       *Inventory  `json:"inventory,omitempty"`
	Locations       *Locations  `json:"locations,omitempty"`
	DraftStocktakes *int64      `json:"draft_stocktakes,omitempty"`
	Activity        *Activity   `json:"activity,omitempty"`
	ImportIssues    *Imports    `json:"import_issues,omitempty"`
}
