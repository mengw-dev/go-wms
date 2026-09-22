// Package api 定义库存模块在调用方事务内执行的数量变更接口。
package api

import (
	"context"

	"gorm.io/gorm"
)

// IncreaseReq 上架入库：stock+N, available+N（四元组存在则累加，不存在则创建）。
type IncreaseReq struct {
	WarehouseID int64
	LocationID  int64
	SKUID       int64
	BatchNo     string
	Quantity    int
	OrderNo     string // 来源单据号
	TaskNo      string
	Operator    string
}

// AllocateReq FIFO 分配：available-N, allocated+N（stock 不变，审核即锁库）。
type AllocateReq struct {
	WarehouseID int64
	SKUID       int64
	Quantity    int
	OrderNo     string
	Operator    string
}

// AllocateRow 分配结果行：指向被锁定的具体库存行。
type AllocateRow struct {
	InventoryID  int64
	LocationID   int64
	LocationCode string
	BatchNo      string
	Quantity     int
}

// AllocateResult 返回本次 FIFO 分配实际占用的库存行和总数量。
type AllocateResult struct {
	Rows  []AllocateRow
	Total int
}

// ShipReq 发货扣减：stock-N, allocated-N。
type ShipReq struct {
	InventoryID int64
	Quantity    int
	OrderNo     string
	TaskNo      string
	Operator    string
}

// ReleaseReq 取消分配释放：available+N, allocated-N。
type ReleaseReq struct {
	InventoryID int64
	Quantity    int
	OrderNo     string
	Operator    string
}

// AdjustReq 盘点调整：将库存账面数调整为 NewStock，同事务写 ADJUST 流水。
type AdjustReq struct {
	InventoryID int64
	NewStock    int
	OrderNo     string
	Operator    string
}

// InventoryAPI inventory 模块对外接口。全部方法要求在调用方事务内执行（tx 由 Service 层传入）。
type InventoryAPI interface {
	Increase(ctx context.Context, tx *gorm.DB, req *IncreaseReq) error
	Allocate(ctx context.Context, tx *gorm.DB, req *AllocateReq) (*AllocateResult, error)
	Ship(ctx context.Context, tx *gorm.DB, req *ShipReq) error
	Release(ctx context.Context, tx *gorm.DB, req *ReleaseReq) error
	// Adjust 返回在行锁内计算并实际应用的库存差异。
	Adjust(ctx context.Context, tx *gorm.DB, req *AdjustReq) (int, error)
}
