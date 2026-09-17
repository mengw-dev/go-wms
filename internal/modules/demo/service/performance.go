package service

import (
	"context"
	"runtime"
	"time"

	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	taskmodel "gowms/internal/modules/task/model"
)

// PerformanceSnapshot HR 性能页使用的实时快照，不依赖 Prometheus 是否启动。
type PerformanceSnapshot struct {
	CheckedAt time.Time       `json:"checked_at"`
	Database  ComponentHealth `json:"database"`
	Redis     ComponentHealth `json:"redis"`
	Pool      DBPoolStats     `json:"pool"`
	Runtime   RuntimeStats    `json:"runtime"`
	Business  BusinessStats   `json:"business"`
}

type ComponentHealth struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Message   string `json:"message,omitempty"`
}

type DBPoolStats struct {
	MaxOpenConnections int   `json:"max_open_connections"`
	OpenConnections    int   `json:"open_connections"`
	InUse              int   `json:"in_use"`
	Idle               int   `json:"idle"`
	WaitCount          int64 `json:"wait_count"`
	WaitDurationMs     int64 `json:"wait_duration_ms"`
}

type RuntimeStats struct {
	Goroutines    int     `json:"goroutines"`
	MemoryAllocMB float64 `json:"memory_alloc_mb"`
	MemorySysMB   float64 `json:"memory_sys_mb"`
	NumGC         uint32  `json:"num_gc"`
}

type BusinessStats struct {
	InboundToday   int64 `json:"inbound_today"`
	OutboundToday  int64 `json:"outbound_today"`
	PendingTasks   int64 `json:"pending_tasks"`
	InventoryRows  int64 `json:"inventory_rows"`
	StockTotal     int64 `json:"stock_total"`
	AvailableTotal int64 `json:"available_total"`
	AllocatedTotal int64 `json:"allocated_total"`
}

// Performance 返回数据库、Redis、连接池、Go 运行时和业务数据快照。
func (s *Service) Performance(ctx context.Context) (*PerformanceSnapshot, error) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, err
	}

	snapshot := &PerformanceSnapshot{CheckedAt: time.Now()}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	start := time.Now()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		snapshot.Database = ComponentHealth{Status: "down", LatencyMs: time.Since(start).Milliseconds(), Message: err.Error()}
	} else {
		snapshot.Database = ComponentHealth{Status: "ok", LatencyMs: time.Since(start).Milliseconds()}
	}

	start = time.Now()
	if err := s.rdb.Ping(pingCtx).Err(); err != nil {
		snapshot.Redis = ComponentHealth{Status: "down", LatencyMs: time.Since(start).Milliseconds(), Message: err.Error()}
	} else {
		snapshot.Redis = ComponentHealth{Status: "ok", LatencyMs: time.Since(start).Milliseconds()}
	}

	stats := sqlDB.Stats()
	snapshot.Pool = DBPoolStats{
		MaxOpenConnections: stats.MaxOpenConnections,
		OpenConnections:    stats.OpenConnections,
		InUse:              stats.InUse,
		Idle:               stats.Idle,
		WaitCount:          stats.WaitCount,
		WaitDurationMs:     stats.WaitDuration.Milliseconds(),
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	snapshot.Runtime = RuntimeStats{
		Goroutines:    runtime.NumGoroutine(),
		MemoryAllocMB: float64(mem.Alloc) / 1024 / 1024,
		MemorySysMB:   float64(mem.Sys) / 1024 / 1024,
		NumGC:         mem.NumGC,
	}

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dayEnd := dayStart.Add(24 * time.Hour)
	if err := s.db.WithContext(ctx).Model(&inboundmodel.ReceiptOrder{}).
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Count(&snapshot.Business.InboundToday).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&outboundmodel.ShipmentOrder{}).
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Count(&snapshot.Business.OutboundToday).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&taskmodel.Task{}).
		Where("status NOT IN ?", []string{"COMPLETED", "CANCELLED"}).
		Count(&snapshot.Business.PendingTasks).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Count(&snapshot.Business.InventoryRows).Error; err != nil {
		return nil, err
	}
	var inventoryTotals struct {
		StockTotal     int64
		AvailableTotal int64
		AllocatedTotal int64
	}
	if err := s.db.WithContext(ctx).Model(&invmodel.Inventory{}).
		Select("COALESCE(SUM(stock_quantity), 0) AS stock_total, COALESCE(SUM(available_quantity), 0) AS available_total, COALESCE(SUM(allocated_quantity), 0) AS allocated_total").
		Scan(&inventoryTotals).Error; err != nil {
		return nil, err
	}
	snapshot.Business.StockTotal = inventoryTotals.StockTotal
	snapshot.Business.AvailableTotal = inventoryTotals.AvailableTotal
	snapshot.Business.AllocatedTotal = inventoryTotals.AllocatedTotal

	return snapshot, nil
}
