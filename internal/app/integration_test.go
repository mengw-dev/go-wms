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
	"gowms/internal/pkg/errcode"
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
	return pushExternalBody(ctx, router, bizNo, map[string]any{
		"biz_order_no": bizNo, "warehouse_code": warehouse, "tenant_id": "22",
		"details": []map[string]any{{"sku_code": sharedFixtureSKU, "expected_qty": 2}},
	})
}

// pushExternalBody 发送可自定义内容的外部出库创建请求，body 已含业务单号。
func pushExternalBody(ctx context.Context, router *gin.Engine, bizNo string, body map[string]any) *httptest.ResponseRecorder {
	body["biz_order_no"] = bizNo
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/integration/outbound-orders?tenant_id=22", bytes.NewReader(payload)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-key")
	req.Header.Set("X-Tenant-ID", "22")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

// externalError 解析失败响应的 HTTP 状态码与业务错误码。
func externalError(t *testing.T, recorder *httptest.ResponseRecorder) (int, int) {
	t.Helper()
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	return recorder.Code, envelope.Code
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

// TestExternalReplaySameContentReturnsOriginalOrder 同号同内容：返回原单、不重复建单。
// 回放核对完全基于持久化的创建快照（warehouse_id/remark/明细编码与数量），
// 因此对历史订单（本测试中上一次请求创建的订单）与并发胜者同样适用。
func TestExternalReplaySameContentReturnsOriginalOrder(t *testing.T) {
	db, _, _ := integrationFixture(t)
	if err := db.Create(&basicmodel.SKU{TenantID: 11, Code: "SKU-REPLAY", Barcode: "BC-REPLAY", Name: "replay", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	router := integrationRouter(t, db, 11)
	ctx := context.Background()
	body := map[string]any{
		"warehouse_code": "SHARED-WH", "remark": "同内容重放",
		"details": []map[string]any{
			{"sku_code": sharedFixtureSKU, "expected_qty": 2},
			{"sku_code": "SKU-REPLAY", "expected_qty": 3},
		},
	}
	first := externalResult(t, pushExternalBody(ctx, router, "REPLAY-SAME", body))
	if first.Idempotent {
		t.Fatalf("first push should create: %+v", first)
	}
	replayed := externalResult(t, pushExternalBody(ctx, router, "REPLAY-SAME", body))
	if !replayed.Idempotent || replayed.OrderID != first.OrderID {
		t.Fatalf("replay result=%+v want ID=%d idempotent=true", replayed, first.OrderID)
	}
	var count int64
	if err := db.Model(&model.ShipmentOrder{}).Where("biz_order_no = ?", "REPLAY-SAME").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("orders=%d err=%v", count, err)
	}
}

// TestExternalDetailOrderDoesNotAffectEquality 明细顺序不影响相等判断：
// 同号同内容但明细行顺序不同，仍回放原单。
func TestExternalDetailOrderDoesNotAffectEquality(t *testing.T) {
	db, _, _ := integrationFixture(t)
	for _, sku := range []basicmodel.SKU{
		{TenantID: 11, Code: "SKU-ORDER-A", Barcode: "BC-ORDER-A", Name: "order a", Status: 1},
		{TenantID: 11, Code: "SKU-ORDER-B", Barcode: "BC-ORDER-B", Name: "order b", Status: 1},
	} {
		if err := db.Create(&sku).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := integrationRouter(t, db, 11)
	ctx := context.Background()
	body := func(details []map[string]any) map[string]any {
		return map[string]any{"warehouse_code": "SHARED-WH", "details": details}
	}
	first := externalResult(t, pushExternalBody(ctx, router, "ORDER-INSENSITIVE", body([]map[string]any{
		{"sku_code": "SKU-ORDER-A", "expected_qty": 2},
		{"sku_code": "SKU-ORDER-B", "expected_qty": 3},
	})))
	if first.Idempotent {
		t.Fatalf("first push should create: %+v", first)
	}
	replayed := externalResult(t, pushExternalBody(ctx, router, "ORDER-INSENSITIVE", body([]map[string]any{
		{"sku_code": "SKU-ORDER-B", "expected_qty": 3},
		{"sku_code": "SKU-ORDER-A", "expected_qty": 2},
	})))
	if !replayed.Idempotent || replayed.OrderID != first.OrderID {
		t.Fatalf("reordered details must replay original: %+v want ID=%d", replayed, first.OrderID)
	}
}

// TestExternalBizNoContentConflict 同号异内容返回 409（50012）：
// 数量变化、SKU 集合变化、备注变化、仓库变化分别核对。
func TestExternalBizNoContentConflict(t *testing.T) {
	db, _, _ := integrationFixture(t)
	if err := db.Create(&basicmodel.SKU{TenantID: 11, Code: "SKU-CONFLICT", Barcode: "BC-CONFLICT", Name: "conflict", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&basicmodel.Warehouse{TenantID: 11, Code: "WH-CONFLICT", Name: "conflict wh", Status: 1}).Error; err != nil {
		t.Fatal(err)
	}
	router := integrationRouter(t, db, 11)
	ctx := context.Background()
	pushFn := func(qty int, skuCodes []string, remark, warehouse string) *httptest.ResponseRecorder {
		details := make([]map[string]any, 0, len(skuCodes))
		for _, code := range skuCodes {
			details = append(details, map[string]any{"sku_code": code, "expected_qty": qty})
		}
		return pushExternalBody(ctx, router, "CONFLICT-BIZ", map[string]any{
			"warehouse_code": warehouse, "remark": remark, "details": details,
		})
	}
	first := externalResult(t, pushFn(2, []string{sharedFixtureSKU, "SKU-CONFLICT"}, "基准备注", "SHARED-WH"))
	if first.Idempotent {
		t.Fatalf("first push should create: %+v", first)
	}
	cases := []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"数量变化", pushFn(7, []string{sharedFixtureSKU, "SKU-CONFLICT"}, "基准备注", "SHARED-WH")},
		{"SKU 集合变化", pushFn(2, []string{sharedFixtureSKU}, "基准备注", "SHARED-WH")},
		{"备注变化", pushFn(2, []string{sharedFixtureSKU, "SKU-CONFLICT"}, "不同备注", "SHARED-WH")},
		{"仓库变化", pushFn(2, []string{sharedFixtureSKU, "SKU-CONFLICT"}, "基准备注", "WH-CONFLICT")},
	}
	for _, tc := range cases {
		if status, code := externalError(t, tc.rec); status != http.StatusConflict || code != errcode.BizOrderContentConflict.Code {
			t.Errorf("%s: status=%d code=%d body=%s", tc.name, status, code, tc.rec.Body.String())
		}
	}
	var count int64
	if err := db.Model(&model.ShipmentOrder{}).Where("biz_order_no = ?", "CONFLICT-BIZ").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("orders=%d err=%v", count, err)
	}
}

// TestExternalCrossTenantSameNumberIndependentContent 不同租户同单号互相独立，
// 允许各自使用不同的创建内容。
func TestExternalCrossTenantSameNumberIndependentContent(t *testing.T) {
	db, _, _ := integrationFixture(t)
	router22 := integrationRouter(t, db, 22)
	router11 := integrationRouter(t, db, 11)
	ctx := context.Background()
	body := func(qty int) map[string]any {
		return map[string]any{
			"warehouse_code": "SHARED-WH",
			"details":        []map[string]any{{"sku_code": sharedFixtureSKU, "expected_qty": qty}},
		}
	}
	a := externalResult(t, pushExternalBody(ctx, router22, "X-TENANT-BIZ", body(2)))
	b := externalResult(t, pushExternalBody(ctx, router11, "X-TENANT-BIZ", body(7)))
	if a.OrderID == b.OrderID || a.Idempotent || b.Idempotent {
		t.Fatalf("cross-tenant same biz no must be independent: %+v %+v", a, b)
	}
	var count int64
	if err := db.Model(&model.ShipmentOrder{}).Where("biz_order_no = ?", "X-TENANT-BIZ").Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("orders=%d err=%v", count, err)
	}
}

// TestExternalConcurrentDifferentContentCreatesSingleOrder 并发同号异内容：
// 数据库唯一键保证仍只有一张订单；后到者回查后因内容不一致得到 409。
func TestExternalConcurrentDifferentContentCreatesSingleOrder(t *testing.T) {
	db, _, _ := integrationFixture(t)
	router := integrationRouter(t, db, 11)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	if err := db.Callback().Create().Before("gorm:create").Register("test:concurrent_external_content", func(query *gorm.DB) {
		order, ok := query.Statement.Dest.(*model.ShipmentOrder)
		if !ok || order.BizOrderNo != "RACING-DIFF" {
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
	for _, qty := range []int{2, 4} {
		go func(q int) {
			done <- pushExternalBody(ctx, router, "RACING-DIFF", map[string]any{
				"warehouse_code": "SHARED-WH",
				"details":        []map[string]any{{"sku_code": sharedFixtureSKU, "expected_qty": q}},
			})
		}(qty)
	}
	for range 2 {
		select {
		case <-arrived:
		case <-ctx.Done():
			t.Fatal("both requests did not reach create after passing duplicate checks")
		}
	}
	close(release)
	var created *dto.ExternalCreateOrderResp
	conflicts := 0
	for range 2 {
		select {
		case rec := <-done:
			status, code := externalError(t, rec)
			if code == errcode.BizOrderContentConflict.Code {
				if status != http.StatusConflict {
					t.Fatalf("conflict status=%d body=%s", status, rec.Body.String())
				}
				conflicts++
				continue
			}
			if code != 0 {
				t.Fatalf("unexpected error code=%d body=%s", code, rec.Body.String())
			}
			result := externalResult(t, rec)
			if result.Idempotent {
				t.Fatalf("winner must be a fresh create: %+v", result)
			}
			created = &result
		case <-ctx.Done():
			t.Fatal("concurrent push timed out")
		}
	}
	if created == nil || conflicts != 1 {
		t.Fatalf("want one create and one conflict, got create=%+v conflicts=%d", created, conflicts)
	}
	var count int64
	if err := db.Model(&model.ShipmentOrder{}).Where("biz_order_no = ?", "RACING-DIFF").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("orders=%d err=%v", count, err)
	}
	var detail model.ShipmentOrderDetail
	if err := db.Where("order_id = ?", created.OrderID).First(&detail).Error; err != nil {
		t.Fatal(err)
	}
	if detail.ExpectedQty != 2 && detail.ExpectedQty != 4 {
		t.Fatalf("winner detail qty=%d unexpected", detail.ExpectedQty)
	}
}

// TestExternalLegacyOrderContentUnverifiableNeedsReview 历史订单无法可靠重建创建内容时，
// 不得当作重复成功放行，必须报告待人工核对（50013）。
// 现 schema 创建期必然写入 SKU 编码快照，此分支是防御性保护：
// 缺失编码快照的历史明细无法与请求内容做可靠比较。
func TestExternalLegacyOrderContentUnverifiableNeedsReview(t *testing.T) {
	db, _, _ := integrationFixture(t)
	router := integrationRouter(t, db, 11)
	ctx := context.Background()
	body := map[string]any{
		"warehouse_code": "SHARED-WH",
		"details":        []map[string]any{{"sku_code": sharedFixtureSKU, "expected_qty": 2}},
	}
	first := externalResult(t, pushExternalBody(ctx, router, "LEGACY-BIZ", body))
	if first.Idempotent {
		t.Fatalf("first push should create: %+v", first)
	}
	if err := db.Model(&model.ShipmentOrderDetail{}).Where("order_id = ?", first.OrderID).
		Update("sku_code", "").Error; err != nil {
		t.Fatal(err)
	}
	rec := pushExternalBody(ctx, router, "LEGACY-BIZ", body)
	if status, code := externalError(t, rec); status != http.StatusConflict || code != errcode.BizOrderContentUnknown.Code {
		t.Fatalf("status=%d code=%d body=%s", status, code, rec.Body.String())
	}
}
