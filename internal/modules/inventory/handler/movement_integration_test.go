package handler

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/dto"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	"gowms/internal/modules/inventory/service"
	"gowms/internal/pkg/middleware"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// Exercise real HTTP binding, DTO string IDs, release DDL and transaction path.
// Authentication identity is injected here; JWT validation has separate tests.
func TestMoveReleaseHTTPReceiptAndReplay(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenMigratedMySQL(t, dsn)
	ctx := tenant.WithTenant(context.Background(), 11)
	if err := snowflake.Init(1); err != nil {
		t.Fatal(err)
	}
	for _, row := range []any{
		&basicmodel.Warehouse{Base: modelbase.Base{ID: 1}, Code: "WH", Name: "仓库", Status: 1},
		&basicmodel.SKU{Base: modelbase.Base{ID: 1}, Code: "SKU", Barcode: "001", Name: "测试货品", Unit: "箱", Status: 1},
		&basicmodel.Location{Base: modelbase.Base{ID: 1}, WarehouseID: 1, Code: "A1", Status: 2},
		&basicmodel.Location{Base: modelbase.Base{ID: 2}, WarehouseID: 1, Code: "A2", Status: 1},
		&model.Inventory{Base: modelbase.Base{ID: 10}, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B1", StockQuantity: 10, AvailableQty: 8, AllocatedQty: 2, StockInTime: time.Now().Truncate(time.Millisecond)},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", int64(42))
		c.Set("username", "worker")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	var checker middleware.PermsChecker = movePermissions{true, true}
	New(service.New(repository.New(), tx.New(db))).RegisterRoutes(r.Group(""), checker)
	request := func(method, path, body, key string) (int, []byte) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", key)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}
	body := `{"source_inventory_id":"10","target_location_id":"2","quantity":3,"reason":"整理"}`
	if status, _ := request("POST", "/inventory/moves", body, ""); status != 400 {
		t.Fatalf("missing key: %d", status)
	}
	var first dto.Movement
	var firstJSON []byte
	for i := range 2 {
		status, data := request("POST", "/inventory/moves", body, "same-key")
		if status != 200 {
			t.Fatalf("move status=%d body=%s", status, data)
		}
		var out struct {
			Code int          `json:"code"`
			Data dto.Movement `json:"data"`
		}
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatal(err)
		}
		if out.Code != 0 || out.Data.ID <= 0 || out.Data.SourceLocationID != 1 || out.Data.TargetLocationID != 2 {
			t.Fatalf("unexpected receipt %s", data)
		}
		if i == 0 {
			first = out.Data
			firstJSON = data
		} else if string(firstJSON) != string(data) {
			t.Fatalf("replay differs\n%s\n%s", firstJSON, data)
		}
	}
	status, data := request("GET", "/inventory/moves?operation_key=same-key", "", "")
	if status != 200 || !strings.Contains(string(data), first.MoveNo) || strings.Contains(string(data), "request_hash") {
		t.Fatalf("history %d %s", status, data)
	}
	status, data = request("GET", "/inventory/move-source/10", "", "")
	if status != 200 || !strings.Contains(string(data), `"available_qty":5`) {
		t.Fatalf("source %d %s", status, data)
	}
	status, data = request("GET", "/inventory/move-locations?source_inventory_id=10", "", "")
	if status != 200 || !strings.Contains(string(data), `"code":"A2"`) || strings.Contains(string(data), `"code":"A1"`) {
		t.Fatalf("options %d %s", status, data)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.InventoryTrans{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("duplicate ledger %d %v", count, err)
	}
}
