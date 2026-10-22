package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	basicmodel "gowms/internal/modules/basic/model"
	inmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outmodel "gowms/internal/modules/outbound/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

type scanPermission struct{ read, putaway bool }

func (p scanPermission) HasPerm(_ context.Context, _ int64, perm string) bool {
	if perm == "wms:inbound:putaway" {
		return p.putaway
	}
	return p.read && (perm == "wms:inbound:view" || perm == "wms:outbound:view")
}

func TestOrderScanningHTTPReadOnlyAndIsolation(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenMigratedMySQL(t, dsn)
	for _, tid := range []int64{22, 0, 11} {
		ctx := tenant.WithExactTenant(context.Background(), tid)
		b := tid * 1000
		rows := []any{
			&basicmodel.Warehouse{Base: modelbase.Base{ID: b + 1}, Code: "WH", Name: "仓库", Status: 1},
			&basicmodel.Warehouse{Base: modelbase.Base{ID: b + 2}, Code: "WH2", Name: "其他仓库", Status: 1},
			&basicmodel.SKU{Base: modelbase.Base{ID: b + 1}, Code: "SKU-A", Barcode: "00123", Name: "货品A", Status: 1},
			&basicmodel.SKU{Base: modelbase.Base{ID: b + 2}, Code: "00123", Barcode: "ALT001", Name: "货品B", Status: 1},
			&basicmodel.SKU{Base: modelbase.Base{ID: b + 3}, Code: "DISABLED", Barcode: "OFF", Name: "禁用货品", Status: 0},
			&basicmodel.SKU{Base: modelbase.Base{ID: b + 4}, Code: "DELETED", Barcode: "GONE", Name: "已删货品", Status: 1},
			&basicmodel.SKU{Base: modelbase.Base{ID: b + 9}, Code: "UNRELATED", Barcode: "OUTSIDE", Name: "未在单据中", Status: 1},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 1}, WarehouseID: b + 1, Code: "A01", Status: 1},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 2}, WarehouseID: b + 1, Code: "A02", Status: 2},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 3}, WarehouseID: b + 1, Code: "OFF", Status: 0},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 4}, WarehouseID: b + 2, Code: "OTHER", Status: 1},
			&basicmodel.Location{Base: modelbase.Base{ID: b + 5}, WarehouseID: b + 1, Code: "GONE", Status: 1},
			&inmodel.ReceiptOrder{Base: modelbase.Base{ID: b + 10}, OrderNo: "RK1", WarehouseID: b + 1, ExpectedQty: 10},
			&outmodel.ShipmentOrder{Base: modelbase.Base{ID: b + 10}, OrderNo: "CK1", BizOrderNo: "BIZ1", WarehouseID: b + 1, ExpectedQty: 10},
		}
		for _, row := range rows {
			if err := db.WithContext(ctx).Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		// GORM applies status=1 defaults to zero-valued struct inserts.
		if err := db.WithContext(ctx).Model(&basicmodel.SKU{}).Where("id=?", b+3).Update("status", 0).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(&basicmodel.Location{}).Where("id=?", b+3).Update("status", 0).Error; err != nil {
			t.Fatal(err)
		}
		for i, sku := range []int64{b + 1, b + 1, b + 2, b + 3, b + 4} {
			for _, row := range []any{
				&inmodel.ReceiptOrderDetail{Base: modelbase.Base{ID: b + int64(i) + 100}, OrderID: b + 10, SKUID: sku, ExpectedQty: 2},
				&outmodel.ShipmentOrderDetail{Base: modelbase.Base{ID: b + int64(i) + 100}, OrderID: b + 10, SKUID: sku, ExpectedQty: 2},
			} {
				if err := db.WithContext(ctx).Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := db.WithContext(ctx).Delete(&basicmodel.SKU{}, b+4).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(ctx).Delete(&basicmodel.Location{}, b+5).Error; err != nil {
			t.Fatal(err)
		}
	}
	// Broken foreign reference must not resolve a different tenant's SKU.
	ctx := tenant.WithTenant(context.Background(), 11)
	if err := db.WithContext(ctx).Create(&inmodel.ReceiptOrderDetail{Base: modelbase.Base{ID: 11999}, OrderID: 11010, SKUID: 22009, ExpectedQty: 1}).Error; err != nil {
		t.Fatal(err)
	}
	a := New(&config.Config{}, db, nil, nil)
	request := func(tid int64, p scanPermission, path string) (int, json.RawMessage) {
		t.Helper()
		gin.SetMode(gin.TestMode)
		r := gin.New()
		g := r.Group("")
		a.InboundHandler.RegisterRoutes(g, p)
		a.OutboundHandler.RegisterRoutes(g, p)
		req := httptest.NewRequest("GET", path, nil).WithContext(tenant.WithTenant(context.Background(), tid))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var body struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return w.Code, body.Data
	}
	for _, module := range []string{"inbound", "outbound"} {
		for _, tc := range []struct {
			code string
			want int
		}{{"SKU-A", 1}, {"00123", 2}, {"ALT001", 1}, {"123", 0}, {"sku-a", 0}, {"OUTSIDE", 0}, {"OFF", 0}, {"GONE", 0}, {"' OR 1=1 --", 0}} {
			t.Run(module+"/"+tc.code, func(t *testing.T) {
				status, data := request(11, scanPermission{true, true}, fmt.Sprintf("/%s/orders/11010/scan-sku?code=%s", module, url.QueryEscape(tc.code)))
				var rows []struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(data, &rows); err != nil {
					t.Fatal(err)
				}
				if status != 200 || len(rows) != tc.want {
					t.Fatalf("status=%d rows=%s", status, data)
				}
				for _, row := range rows {
					if row.ID != "11001" && row.ID != "11002" {
						t.Fatalf("foreign SKU %s", data)
					}
				}
			})
		}
		t.Run(module+"/platform-exact", func(t *testing.T) {
			status, data := request(0, scanPermission{true, true}, fmt.Sprintf("/%s/orders/10/scan-sku?code=SKU-A", module))
			if status != 200 || string(data) != `[{"id":"1","code":"SKU-A","name":"货品A","barcode":"00123"}]` {
				t.Fatalf("platform %d %s", status, data)
			}
		})
		for _, tid := range []int64{11, 0} {
			t.Run(fmt.Sprintf("%s/foreign/%d", module, tid), func(t *testing.T) {
				status, _ := request(tid, scanPermission{true, true}, fmt.Sprintf("/%s/orders/22010/scan-sku?code=SKU-A", module))
				if status != 404 {
					t.Fatalf("foreign order status=%d", status)
				}
			})
		}
	}
	for _, tc := range []struct {
		query string
		want  int64
	}{{"", 2}, {"&code=A02", 1}, {"&code=OTHER", 0}, {"&code=OFF", 0}, {"&code=GONE", 0}, {"&code=a01", 0}, {"&keyword=A%25", 0}, {"&page_size=1&page=2", 2}} {
		t.Run("locations/"+tc.query, func(t *testing.T) {
			status, data := request(11, scanPermission{true, true}, "/inbound/orders/11010/putaway-locations?"+strings.TrimPrefix(tc.query, "&"))
			var body struct {
				Total int64 `json:"total"`
				List  []struct {
					ID string `json:"id"`
				} `json:"list"`
			}
			if err := json.Unmarshal(data, &body); err != nil {
				t.Fatal(err)
			}
			if status != 200 || body.Total != tc.want {
				t.Fatalf("locations %d %s", status, data)
			}
			for _, row := range body.List {
				if row.ID != "11001" && row.ID != "11002" {
					t.Fatalf("foreign location %s", data)
				}
			}
		})
	}
	for _, tc := range []struct {
		path string
		p    scanPermission
		want int
	}{
		{"/inbound/orders/11010/scan-sku?code=SKU-A", scanPermission{}, 403},
		{"/outbound/orders/11010/scan-sku?code=SKU-A", scanPermission{}, 403},
		{"/inbound/orders/11010/putaway-locations", scanPermission{read: true}, 403},
		{"/inbound/orders/11010/scan-sku", scanPermission{read: true}, 400},
		{"/outbound/orders/11010/scan-sku?code=%00", scanPermission{read: true}, 400},
		{"/inbound/orders/11010/putaway-locations?page_size=101", scanPermission{true, true}, 400},
		{"/inbound/orders/22010/putaway-locations", scanPermission{true, true}, 404},
	} {
		t.Run(tc.path, func(t *testing.T) {
			status, _ := request(11, tc.p, tc.path)
			if status != tc.want {
				t.Fatalf("status=%d want=%d", status, tc.want)
			}
		})
	}
	for _, m := range []any{&invmodel.Inventory{}, &invmodel.InventoryTrans{}} {
		var count int64
		if err := db.WithContext(ctx).Model(m).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("scan wrote stock: %d %v", count, err)
		}
	}
}
