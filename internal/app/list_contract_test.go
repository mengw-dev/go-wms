package app

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	basicmodel "gowms/internal/modules/basic/model"
	inhandler "gowms/internal/modules/inbound/handler"
	inmodel "gowms/internal/modules/inbound/model"
	inrepo "gowms/internal/modules/inbound/repository"
	inservice "gowms/internal/modules/inbound/service"
	invhandler "gowms/internal/modules/inventory/handler"
	invmodel "gowms/internal/modules/inventory/model"
	invrepo "gowms/internal/modules/inventory/repository"
	invservice "gowms/internal/modules/inventory/service"
	outhandler "gowms/internal/modules/outbound/handler"
	outmodel "gowms/internal/modules/outbound/model"
	outrepo "gowms/internal/modules/outbound/repository"
	outservice "gowms/internal/modules/outbound/service"
	taskhandler "gowms/internal/modules/task/handler"
	taskmodel "gowms/internal/modules/task/model"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

type listContractPermission struct{}

func (listContractPermission) HasPerm(context.Context, int64, string) bool { return true }

// Test the real HTTP parameter/DTO/service/SQL/response path. Authentication is
// deliberately outside this fixture; tenant callbacks and filters are real.
func TestBusinessListContracts(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &inmodel.ReceiptOrder{}, &outmodel.ShipmentOrder{}, &taskmodel.Task{}, &basicmodel.SKU{}, &invmodel.Inventory{}, &invmodel.InventoryTrans{})
	ctx := tenant.WithTenant(context.Background(), 11)
	stamp := time.Date(2026, 1, 2, 12, 0, 0, 0, time.Local)
	seed := func(ctx context.Context, row any) {
		t.Helper()
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for n := int64(1); n <= 7; n++ {
		rowCtx := ctx
		wh, status, word := int64(21), "DRAFT", "MATCH"
		if n == 3 {
			wh = 22
		}
		if n == 4 {
			status = "SUBMITTED"
		}
		if n == 5 {
			word = "OTHER"
		}
		if n == 7 {
			rowCtx = tenant.WithTenant(ctx, 22)
		}
		base := modelbase.Base{ID: n, CreatedAt: stamp, UpdatedAt: stamp}
		in := &inmodel.ReceiptOrder{Base: base, OrderNo: "RK-" + word + "-" + strconv.FormatInt(n, 10), WarehouseID: wh, Status: inmodel.OrderStatus(status)}
		out := &outmodel.ShipmentOrder{Base: base, OrderNo: "CK-" + word + "-" + strconv.FormatInt(n, 10), BizOrderNo: "BIZ-" + strconv.FormatInt(n, 10), WarehouseID: wh, Status: outmodel.OrderStatus(status)}
		seed(rowCtx, in)
		seed(rowCtx, out)
		if n == 6 {
			if err := db.WithContext(ctx).Delete(in).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.WithContext(ctx).Delete(out).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for n := int64(1); n <= 7; n++ {
		rowCtx := ctx
		wh, kind, status := int64(21), taskmodel.TaskPick, taskmodel.TaskCreated
		if n == 3 {
			wh = 22
		}
		if n == 4 {
			kind = taskmodel.TaskReceive
		}
		if n == 5 {
			status = taskmodel.TaskCompleted
		}
		if n == 6 {
			rowCtx = tenant.WithTenant(ctx, 22)
		}
		row := &taskmodel.Task{Base: modelbase.Base{ID: n}, TaskNo: "PK-" + strconv.FormatInt(n, 10), OrderNo: "ROOT%_", WarehouseID: wh, OrderID: 901, TaskType: kind, Status: status, TargetQty: 1}
		seed(rowCtx, row)
		if n == 7 {
			if err := db.WithContext(ctx).Delete(row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(ctx, &basicmodel.SKU{Base: modelbase.Base{ID: 501}, Code: "SKU%_501", Name: "Water", Barcode: "BC501", Status: 1})
	seed(ctx, &basicmodel.SKU{Base: modelbase.Base{ID: 502}, Code: "SKU502", Name: "Empty", Barcode: "BC502", Status: 1})
	for n := int64(1); n <= 5; n++ {
		rowCtx := ctx
		wh, sku, qty := int64(21), int64(501), 10
		if n == 2 {
			wh, qty = 22, 20
		}
		if n == 3 {
			sku, qty = 502, 0
		}
		if n == 4 {
			rowCtx = tenant.WithTenant(ctx, 22)
		}
		row := &invmodel.Inventory{Base: modelbase.Base{ID: n}, WarehouseID: wh, LocationID: n, SKUID: sku, BatchNo: "B", StockQuantity: qty, AvailableQty: qty, StockInTime: stamp}
		seed(rowCtx, row)
		seed(rowCtx, &invmodel.InventoryTrans{ID: n, InventoryID: n, TransType: invmodel.TransReceive, OrderNo: "RK%_", CreatedAt: stamp})
		if n == 5 {
			if err := db.WithContext(ctx).Delete(row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	tm := tx.New(db)
	inv := invservice.New(invrepo.New(), tm)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	perms := listContractPermission{}
	inhandler.New(inservice.New(inrepo.New(), tm, nil, nil, inv, nil, config.UploadConfig{}, config.LimitsConfig{})).RegisterRoutes(group, perms)
	outhandler.New(outservice.New(outrepo.New(), tm, nil, nil, inv, nil, config.LimitsConfig{})).RegisterRoutes(group, perms)
	taskhandler.New(taskservice.New(taskrepo.New(), db)).RegisterRoutes(group, perms)
	invhandler.New(inv).RegisterRoutes(group, perms)
	cases := []struct {
		name, path, ids string
		total           int64
	}{}
	for _, module := range []string{"inbound", "outbound"} {
		root := "/api/" + module + "/orders"
		for _, row := range []struct {
			name, query, ids string
			total            int64
		}{
			{"combined", "keyword=MATCH&warehouse_id=21&status=DRAFT&page_size=1", "2", 2},
			{"second_page", "keyword=MATCH&warehouse_id=21&status=DRAFT&page_size=1&page=2", "1", 2},
			{"empty", "keyword=MISSING", "", 0},
			{"date_hit", "keyword=MATCH&created_at_from=2026-01-02%2000:00:00&created_at_to=2026-01-02%2023:59:59", "4,3,2,1", 4},
			{"date_miss", "created_at_from=2026-01-03%2000:00:00", "", 0},
			{"literal_wildcard", "keyword=%25", "", 0},
		} {
			cases = append(cases, struct {
				name, path, ids string
				total           int64
			}{module + "_" + row.name, root + "?" + row.query, row.ids, row.total})
		}
	}
	cases = append(cases, []struct {
		name, path, ids string
		total           int64
	}{
		{"task_combined", "/api/tasks?keyword=ROOT&warehouse_id=21&task_type=PICK&status=CREATED&order_id=901&page_size=1", "2", 2},
		{"task_page", "/api/tasks?keyword=ROOT&warehouse_id=21&task_type=PICK&status=CREATED&order_id=901&page_size=1&page=2", "1", 2},
		{"task_no", "/api/tasks?keyword=PK-2", "2", 1},
		{"task_literal", "/api/tasks?keyword=%25_&warehouse_id=21", "5,4,2,1", 4},
		{"task_missing", "/api/tasks?keyword=MISSING", "", 0},
		{"inventory_in_stock", "/api/inventory?warehouse_id=21&in_stock_only=true", "1", 1},
		{"inventory_zero", "/api/inventory?warehouse_id=21&in_stock_only=false", "3,1", 2},
		{"inventory_name", "/api/inventory?sku_keyword=Water&page_size=1&page=2", "1", 2},
		{"inventory_code", "/api/inventory?sku_keyword=%25_", "2,1", 2},
		{"inventory_barcode", "/api/inventory?sku_keyword=BC502", "3", 1},
		{"inventory_ids", "/api/inventory?warehouse_id=21&location_id=1&sku_id=501", "1", 1},
		{"inventory_missing", "/api/inventory?sku_keyword=MISSING", "", 0},
		{"trans_combined", "/api/inventory/trans?inventory_id=1&order_no=%25_&trans_type=RECEIVE", "1", 1},
		{"trans_page", "/api/inventory/trans?order_no=%25_&page_size=1&page=2", "3", 4},
		{"trans_wrong_type", "/api/inventory/trans?trans_type=SHIP", "", 0},
	}...)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", tc.path, nil).WithContext(ctx))
			var body struct {
				Code int `json:"code"`
				Data struct {
					List []struct {
						ID string `json:"id"`
					} `json:"list"`
					Total int64 `json:"total"`
				} `json:"data"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(body.Data.List))
			for _, row := range body.Data.List {
				ids = append(ids, row.ID)
			}
			if recorder.Code != 200 || body.Code != 0 || body.Data.Total != tc.total || strings.Join(ids, ",") != tc.ids {
				t.Fatalf("status=%d body=%s want=%s total=%d", recorder.Code, recorder.Body.String(), tc.ids, tc.total)
			}
		})
	}
	t.Run("summary_groups_and_quantities", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/inventory/summary?page_size=1", nil).WithContext(ctx))
		var body struct {
			Code int `json:"code"`
			Data struct {
				Total int64 `json:"total"`
				List  []struct {
					SKUID string `json:"sku_id"`
					Stock int    `json:"stock_quantity"`
				} `json:"list"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if recorder.Code != 200 || body.Code != 0 || body.Data.Total != 2 || len(body.Data.List) != 1 || body.Data.List[0].SKUID != "501" || body.Data.List[0].Stock != 30 {
			t.Fatalf("summary=%s", recorder.Body.String())
		}
	})
}
