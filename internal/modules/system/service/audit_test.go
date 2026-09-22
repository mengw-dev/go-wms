package service

import (
	"context"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/modules/system/model"
	"gowms/internal/modules/system/repository"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/tenant"
)

func TestOperLogsDrainOnShutdownAndKeepTenant(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN: "test@tcp(127.0.0.1:1)/unused", SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	var written []model.SysOperLog
	if err := db.Callback().Create().Replace("gorm:create", func(db *gorm.DB) {
		if err := db.Statement.Context.Err(); err != nil {
			t.Errorf("flush used canceled context: %v", err)
		}
		rows := db.Statement.ReflectValue
		for i := 0; i < rows.Len(); i++ {
			written = append(written, *rows.Index(i).Interface().(*model.SysOperLog))
		}
	}); err != nil {
		t.Fatal(err)
	}

	s := New(repository.New(db), "unused", 1)
	for i := range 205 {
		ctx := tenant.WithTenant(context.Background(), int64(i%2+1))
		s.Record(ctx, middleware.OperLogRecord{UserID: int64(i + 1), Path: "/test"})
	}
	// 构造函数不启动 goroutine；关闭时必须排空多个批次以及最后不足一批的日志。
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.RunOperLogs(ctx)
	if len(written) != 205 {
		t.Fatalf("written=%d, want 205", len(written))
	}
	for i, row := range written {
		if row.TenantID != int64(i%2+1) || row.UserID != int64(i+1) {
			t.Fatalf("log %d: %+v", i, row)
		}
	}
}
