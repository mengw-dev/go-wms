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
}
