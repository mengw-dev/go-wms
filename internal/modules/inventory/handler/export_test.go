package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/inventory/model"
	"gowms/internal/modules/inventory/repository"
	"gowms/internal/modules/inventory/service"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func TestExportReadPermissionsAndBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		path    string
		allowed bool
		status  int
	}{
		{"/inventory/export", false, 403}, {"/inventory/trans/export", false, 403},
		{"/inventory/export?warehouse_id=-1", true, 400}, {"/inventory/export?in_stock_only=wrong", true, 400},
		{"/inventory/trans/export?trans_type=UNKNOWN", true, 400}, {"/inventory/trans/export?inventory_id=-1", true, 400},
	} {
		t.Run(tc.path, func(t *testing.T) {
			r := gin.New()
			New(nil).RegisterRoutes(r.Group(""), alertPermissions{read: tc.allowed})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			if w.Header().Get("Content-Disposition") != "" {
				t.Fatal("error disguised as download")
			}
		})
	}
}

// Real Gin -> service -> isolated MySQL -> XLSX. The permission checker is a
// read-only test identity, not a replacement for JWT/session middleware tests.
func TestInventoryDownloadHTTPWorkbook(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=True"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.Inventory{}, &model.InventoryTrans{}, &basicmodel.Warehouse{}, &basicmodel.Location{}, &basicmodel.SKU{})
	ctx := tenant.WithTenant(context.Background(), 11)
	for _, row := range []any{
		&basicmodel.SKU{Base: modelbase.Base{ID: 1}, Code: "000123", Barcode: "00123", Name: "Name", Unit: "件"},
		&model.Inventory{Base: modelbase.Base{ID: 1}, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "0001", StockInTime: time.Now()},
		&model.Inventory{Base: modelbase.Base{ID: 2}, WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "0002", StockInTime: time.Now()},
		&model.InventoryTrans{ID: 1, InventoryID: 1, TransType: model.TransOpening, CreatedAt: time.Now()},
	} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Request = c.Request.WithContext(ctx); c.Next() })
	New(service.New(repository.New(), tx.New(db))).RegisterRoutes(r.Group(""), alertPermissions{read: true})
	for _, tc := range []struct {
		path string
		rows int
	}{
		{"/inventory/export?sku_keyword=00123&in_stock_only=false&page=2&page_size=1", 3},
		{"/inventory/trans/export?inventory_id=1&trans_type=OPENING", 2},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != 200 || w.Header().Get("Cache-Control") != "private, no-store" {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
			book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			defer book.Close()
			rows, err := book.GetRows("数据")
			if err != nil || len(rows) != tc.rows {
				t.Fatalf("rows=%v err=%v", rows, err)
			}
		})
	}
	var inventories, ledger int64
	if err := db.WithContext(ctx).Model(&model.Inventory{}).Count(&inventories).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&model.InventoryTrans{}).Count(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if inventories != 2 || ledger != 1 {
		t.Fatal("download changed business records")
	}
}
