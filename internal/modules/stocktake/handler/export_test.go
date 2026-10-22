package handler

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"gowms/internal/modules/stocktake/model"
	"gowms/internal/modules/stocktake/repository"
	"gowms/internal/modules/stocktake/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

type exportPermission bool

func (p exportPermission) HasPerm(_ context.Context, _ int64, perm string) bool {
	return bool(p) && perm == "wms:stocktake:view"
}
func TestExportReadPermissionAndBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		query   string
		allowed bool
		status  int
	}{
		{"", false, 403}, {"?order_id=-1", true, 400}, {"?warehouse_id=-1", true, 400}, {"?status=UNKNOWN", true, 400}, {"?differences_only=no", true, 400},
	} {
		t.Run(tc.query, func(t *testing.T) {
			r := gin.New()
			New(nil).RegisterRoutes(r.Group(""), exportPermission(tc.allowed))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest("GET", "/stocktake/orders/export"+tc.query, nil))
			if w.Code != tc.status {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestStocktakeDownloadHTTPWorkbook(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=True"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.StocktakeOrder{}, &model.StocktakeDetail{})
	ctx := tenant.WithTenant(context.Background(), 11)
	actual := 0
	for _, row := range []any{&model.StocktakeOrder{Base: modelbase.Base{ID: 1}, WarehouseID: 1, OrderNo: "PD001", Status: model.OrderDraft}, &model.StocktakeDetail{Base: modelbase.Base{ID: 2}, OrderID: 1, InventoryID: 1, SKUCode: "00123", BookQty: 10, ActualQty: &actual}} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Request = c.Request.WithContext(ctx); c.Next() })
	New(service.New(repository.New(), tx.New(db), nil, nil, config.LimitsConfig{})).RegisterRoutes(r.Group(""), exportPermission(true))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/stocktake/orders/export?differences_only=true&order_id=1", nil))
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	book, err := excelize.OpenReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	actualCell, err := book.GetCellValue("数据", "L2")
	if err != nil || actualCell != "0" {
		t.Fatalf("zero=%q err=%v", actualCell, err)
	}
	delta, err := book.GetCellValue("数据", "M2")
	if err != nil || delta != "-10" {
		t.Fatalf("variance=%q err=%v", delta, err)
	}
}
