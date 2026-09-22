package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/outbound/dto"
	"gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

const sharedFixtureSKU = "SHARED-SKU"

func integrationFixture(t *testing.T) (*gorm.DB, map[int64]int64, map[int64]int64) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &basicmodel.Warehouse{}, &basicmodel.SKU{}, &model.ShipmentOrder{}, &model.ShipmentOrderDetail{})
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatal(err)
	}
	warehouses, skus := make(map[int64]int64), make(map[int64]int64)
	// 外租户先插入，确保缺失租户条件的 First 会选中错误对象。
	for _, id := range []int64{22, 0, 11} {
		w := basicmodel.Warehouse{TenantID: id, Code: "SHARED-WH", Name: fmt.Sprintf("warehouse-%d", id), Status: 1}
		sku := basicmodel.SKU{TenantID: id, Code: sharedFixtureSKU, Barcode: "SHARED-BC", Name: fmt.Sprintf("sku-%d", id), Status: 1}
		if err := db.Create(&w).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Create(&sku).Error; err != nil {
			t.Fatal(err)
		}
		warehouses[id], skus[id] = w.ID, sku.ID
	}
	return db, warehouses, skus
}

func integrationRouter(t *testing.T, db *gorm.DB, tenantID int64) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a := New(&config.Config{Integration: config.IntegrationConfig{APIKey: "test-key", TenantID: tenantID}}, db, nil, nil)
	router, err := a.NewRouter()
	if err != nil {
		t.Fatal(err)
	}
	return router
}

func pushExternal(ctx context.Context, router *gin.Engine, bizNo, warehouse string) *httptest.ResponseRecorder {
	// 客户端伪造租户信息也不能改变服务端 API Key 的绑定。
	body, _ := json.Marshal(map[string]any{
		"biz_order_no": bizNo, "warehouse_code": warehouse, "tenant_id": "22",
		"details": []map[string]any{{"sku_code": sharedFixtureSKU, "expected_qty": 2}},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integration/outbound-orders?tenant_id=22", bytes.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("X-Tenant-ID", "22")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func externalResult(t *testing.T, recorder *httptest.ResponseRecorder) dto.ExternalCreateOrderResp {
	t.Helper()
	var envelope struct {
		Code int                         `json:"code"`
		Data dto.ExternalCreateOrderResp `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || envelope.Code != 0 {
		t.Fatalf("push failed: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	return envelope.Data
}

func TestExternalAPIKeyIsolatesCodesAndBusinessNumbers(t *testing.T) {
	db, warehouses, skus := integrationFixture(t)
	seen := make(map[int64]bool)
	for _, id := range []int64{22, 0, 11} {
		router := integrationRouter(t, db, id)
		first := externalResult(t, pushExternal(context.Background(), router, "SAME-BIZ-NO", "SHARED-WH"))
		if first.Idempotent || seen[first.OrderID] {
			t.Fatalf("tenant %d received another tenant's order: %+v", id, first)
		}
		seen[first.OrderID] = true
		var order model.ShipmentOrder
		if err := db.First(&order, first.OrderID).Error; err != nil {
			t.Fatal(err)
		}
		var details []model.ShipmentOrderDetail
		if err := db.Where("order_id = ?", order.ID).Find(&details).Error; err != nil {
			t.Fatal(err)
		}
		if order.TenantID != id || order.WarehouseID != warehouses[id] || len(details) != 1 || details[0].TenantID != id || details[0].SKUID != skus[id] {
			t.Fatalf("cross-tenant data: order=%+v details=%+v", order, details)
		}
		repeated := externalResult(t, pushExternal(context.Background(), router, "SAME-BIZ-NO", "SHARED-WH"))
		if !repeated.Idempotent || repeated.OrderID != first.OrderID {
			t.Fatalf("idempotent result=%+v want ID=%d", repeated, first.OrderID)
		}
	}
	// 租户 0 不得回退到别的租户查找只在对方存在的编码。
	if err := db.Create(&basicmodel.Warehouse{TenantID: 22, Code: "FOREIGN-ONLY", Name: "foreign", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	rec := pushExternal(context.Background(), integrationRouter(t, db, 0), "MISSING-WH", "FOREIGN-ONLY")
	if rec.Code == http.StatusOK {
		t.Fatalf("foreign warehouse accepted: %s", rec.Body.String())
	}
}

func TestExternalConcurrentDuplicateReturnsExistingOrder(t *testing.T) {
	db, _, _ := integrationFixture(t)
	router := integrationRouter(t, db, 11)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:concurrent_external", func(query *gorm.DB) {
		order, ok := query.Statement.Dest.(*model.ShipmentOrder)
		if !ok || order.BizOrderNo != "RACING-BIZ" {
			return
		}
		arrived <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			_ = query.AddError(ctx.Err())
		}
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { done <- pushExternal(ctx, router, "RACING-BIZ", "SHARED-WH") }()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("both requests did not reach create after passing duplicate checks")
		}
	}
	close(release)
	var results []dto.ExternalCreateOrderResp
	for range 2 {
		select {
		case rec := <-done:
			results = append(results, externalResult(t, rec))
		case <-ctx.Done():
			t.Fatal("concurrent push timed out")
		}
	}
	if results[0].OrderID != results[1].OrderID || results[0].Idempotent == results[1].Idempotent {
		t.Fatalf("expected one creation and one replay: %+v", results)
	}
	var count int64
	if err := db.Model(&model.ShipmentOrder{}).Where("biz_order_no = ?", "RACING-BIZ").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("orders=%d err=%v", count, err)
	}
}
