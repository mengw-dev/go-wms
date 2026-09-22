package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"gowms/internal/modules/basic/dto"
	"gowms/internal/modules/basic/model"
	"gowms/internal/modules/basic/repository"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
)

type memoryRedis map[string]string

func (r memoryRedis) Get(_ context.Context, key string) (string, error) { return r[key], nil }
func (r memoryRedis) Set(_ context.Context, key string, value any, _ time.Duration) error {
	r[key] = value.(string)
	return nil
}
func (r memoryRedis) Del(_ context.Context, keys ...string) error {
	for _, key := range keys {
		delete(r, key)
	}
	return nil
}

// 用 GORM 查询回调注入结果，测试 Service 的缓存/错误行为，不连接外部数据库。
func newQueryService(t *testing.T, cache redisClient, query func(*gorm.DB)) *Service {
	t.Helper()
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
	if err := db.Callback().Query().Replace("gorm:query", query); err != nil {
		t.Fatal(err)
	}
	return New(repository.New(), tx.New(db), cache, nil, config.LimitsConfig{})
}

func TestBarcodeCacheIsolatesTenants(t *testing.T) {
	cache := memoryRedis{}
	queries := 0
	s := newQueryService(t, cache, func(db *gorm.DB) {
		queries++
		id := tenant.FromContext(db.Statement.Context)
		*db.Statement.Dest.(*model.SKU) = model.SKU{TenantID: id, Barcode: "shared", Name: "SKU"}
	})
	for range 2 {
		for _, id := range []int64{101, 202} {
			sku, err := s.GetByBarcode(tenant.WithTenant(context.Background(), id), "shared")
			if err != nil || sku.TenantID != id {
				t.Fatalf("tenant %d: sku=%+v err=%v", id, sku, err)
			}
		}
	}
	if queries != 2 {
		t.Fatalf("database queries=%d, want 2 (one per tenant)", queries)
	}
	// 错误租户的数据即使出现在当前 key 下，也不能直接返回。
	wrong, err := json.Marshal(model.SKU{TenantID: 202, Barcode: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	cache[barcodeKey(101, "shared")] = string(wrong)
	sku, err := s.GetByBarcode(tenant.WithTenant(context.Background(), 101), "shared")
	if err != nil || sku.TenantID != 101 || queries != 3 {
		t.Fatalf("invalid cache was accepted: sku=%+v queries=%d err=%v", sku, queries, err)
	}
}

func TestUpdateSKUInvalidatesUnchangedBarcode(t *testing.T) {
	cache := memoryRedis{barcodeKey(101, "same"): "old", barcodeKey(202, "same"): "other tenant"}
	s := newQueryService(t, cache, func(db *gorm.DB) {
		*db.Statement.Dest.(*model.SKU) = model.SKU{TenantID: 101, Barcode: "same", Name: "old name"}
	})
	ctx := tenant.WithTenant(context.Background(), 101)
	if err := s.UpdateSKU(ctx, 1, &dto.SKUReq{Code: "sku", Barcode: "same", Name: "new name"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache[barcodeKey(101, "same")]; ok {
		t.Fatal("stale cache was not invalidated")
	}
	if cache[barcodeKey(202, "same")] != "other tenant" {
		t.Fatal("another tenant's cache was removed")
	}
	s.rdb = nil
	if err := s.UpdateSKU(ctx, 1, &dto.SKUReq{Code: "sku", Barcode: "same"}); err != nil {
		t.Fatal(err)
	}
}

func TestSKUQueriesPreserveDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	for _, queryErr := range []error{dbErr, context.Canceled, gorm.ErrRecordNotFound} {
		s := newQueryService(t, nil, func(db *gorm.DB) { _ = db.AddError(queryErr) })
		_, err := s.GetByBarcode(context.Background(), "code")
		want := queryErr
		if errors.Is(queryErr, gorm.ErrRecordNotFound) {
			want = errcode.SKUNotFound
		}
		if !errors.Is(err, want) {
			t.Fatalf("query error %v: got %v, want %v", queryErr, err, want)
		}
	}
}
