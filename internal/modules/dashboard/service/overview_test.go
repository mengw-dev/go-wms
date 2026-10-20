package service_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	basicmodel "gowms/internal/modules/basic/model"
	"gowms/internal/modules/dashboard/dto"
	"gowms/internal/modules/dashboard/handler"
	"gowms/internal/modules/dashboard/repository"
	"gowms/internal/modules/dashboard/service"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

var all = dto.Access{Inbound: true, Outbound: true, Tasks: true, Inventory: true, Basic: true, Stocktake: true}

func fixture(t *testing.T) (*gorm.DB, *service.Service, time.Time) {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &basicmodel.Warehouse{}, &basicmodel.Location{}, &invmodel.Inventory{},
		&inboundmodel.ReceiptOrder{}, &outboundmodel.ShipmentOrder{}, &taskmodel.Task{}, &stocktakemodel.StocktakeOrder{})
	now := time.Date(2026, 10, 20, 16, 30, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600))
	for _, tid := range []int64{0, 99, 77} {
		q := db.WithContext(tenant.WithExactTenant(context.Background(), tid))
		for _, row := range []any{
			&basicmodel.Warehouse{Code: "WH01", Name: "当前仓库", Status: 1},
			&basicmodel.Location{WarehouseID: 1, Code: "A01", Status: 2},
			&basicmodel.Location{WarehouseID: 1, Code: "A02", Status: 1},
			&basicmodel.Location{WarehouseID: 1, Code: "A03", Status: 0},
			&basicmodel.Location{WarehouseID: 2, Code: "B01", Status: 2},
			&invmodel.Inventory{WarehouseID: 1, LocationID: 1, SKUID: 1, BatchNo: "B1", StockInTime: now, StockQuantity: 10, AvailableQty: 8, AllocatedQty: 2},
			&invmodel.Inventory{WarehouseID: 2, LocationID: 2, SKUID: 2, BatchNo: "B2", StockInTime: now, StockQuantity: 10, AvailableQty: 10},
			&stocktakemodel.StocktakeOrder{OrderNo: "PD1", WarehouseID: 1, Status: "DRAFT"},
			&taskmodel.Task{Base: modelbase.Base{CreatedAt: now.Add(-25 * time.Hour)}, TaskNo: "T1", TaskType: "PUTAWAY", Status: "CREATED", WarehouseID: 1, OrderID: 1, TargetQty: 10, ClaimToken: "must-not-leak"},
			&taskmodel.Task{Base: modelbase.Base{CreatedAt: now}, TaskNo: "T2", TaskType: "PICK", Status: "IN_PROGRESS", WarehouseID: 2, OrderID: 2, TargetQty: 10},
			&taskmodel.Task{Base: modelbase.Base{CreatedAt: now.Add(-48 * time.Hour)}, TaskNo: "T3", TaskType: "PICK", Status: "COMPLETED", WarehouseID: 1, OrderID: 3, TargetQty: 10, DoneQty: 10},
		} {
			if err := q.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		for i, created := range []time.Time{now.Add(-time.Hour), now.Add(-6 * 24 * time.Hour), now.Add(-7 * 24 * time.Hour), now.Add(24 * time.Hour)} {
			row := &inboundmodel.ReceiptOrder{Base: modelbase.Base{CreatedAt: created}, OrderNo: string(rune('A' + i)), WarehouseID: 1, Status: "DRAFT"}
			if err := q.Create(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		deleted := &inboundmodel.ReceiptOrder{Base: modelbase.Base{CreatedAt: now}, OrderNo: "DELETED", WarehouseID: 1, Status: "DRAFT"}
		if err := q.Model(&basicmodel.Location{}).Where("code = ?", "A03").Update("status", 0).Error; err != nil {
			t.Fatal(err)
		}
		if err := q.Create(deleted).Error; err != nil {
			t.Fatal(err)
		}
		if err := q.Delete(deleted).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range []any{
			&basicmodel.Warehouse{Code: "DELETED", Name: "已删除仓库", Status: 1},
			&basicmodel.Location{WarehouseID: 1, Code: "DELETED", Status: 2},
			&invmodel.Inventory{WarehouseID: 1, LocationID: 99, SKUID: 99, BatchNo: "DELETED", StockInTime: now, StockQuantity: 10, AvailableQty: 10},
			&stocktakemodel.StocktakeOrder{OrderNo: "DELETED", WarehouseID: 1, Status: "DRAFT"},
			&taskmodel.Task{Base: modelbase.Base{CreatedAt: now.Add(-48 * time.Hour)}, TaskNo: "DELETED", TaskType: "PUTAWAY", Status: "CREATED", WarehouseID: 1, OrderID: 99, TargetQty: 10},
		} {
			if err := q.Create(row).Error; err != nil {
				t.Fatal(err)
			}
			if err := q.Delete(row).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := q.Create(&outboundmodel.ShipmentOrder{Base: modelbase.Base{CreatedAt: now}, OrderNo: "CK1", BizOrderNo: "ERP1", WarehouseID: 2, Status: "SHIPPED"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db, service.New(repository.New(db)), now
}

func TestOverviewTenantWarehouseTimeAndSoftDeletion(t *testing.T) {
	_, svc, now := fixture(t)
	for _, tid := range []int64{0, 99, 77} {
		result, err := svc.Overview(tenant.WithExactTenant(context.Background(), tid), 1, all, now.UTC())
		if err != nil {
			t.Fatal(err)
		}
		if result.TodayFrom != "2026-10-20 00:00:00" || result.TrendFrom != "2026-10-14 00:00:00" {
			t.Fatalf("business date bounds: %+v", result)
		}
		if len(result.Inbound.Statuses) != 1 || result.Inbound.Statuses[0].Total != 4 || result.Inbound.Statuses[0].Today != 1 {
			t.Fatalf("tenant=%d status totals=%+v", tid, result.Inbound.Statuses)
		}
		if len(result.Inbound.Days) != 2 || result.Inbound.Days[0].Date != "2026-10-14" {
			t.Fatalf("trend includes outside date/tenant: %+v", result.Inbound.Days)
		}
		if len(result.Outbound.Recent) != 0 || len(result.OldestTasks) != 1 || result.TaskCounts[0].Older24h != 1 {
			t.Fatalf("warehouse/task filter failed: %+v", result)
		}
		if result.Inventory.SKUs != 1 || result.Inventory.Rows != 1 || result.Inventory.AllocatedRows != 1 {
			t.Fatalf("inventory=%+v", result.Inventory)
		}
		if result.Locations.Enabled != 2 || result.Locations.Occupied != 1 || *result.DraftStocktakes != 1 || len(result.Warehouses) != 1 {
			t.Fatalf("location/stocktake scope failed: %+v", result)
		}
		body, _ := json.Marshal(result)
		if strings.Contains(string(body), "must-not-leak") || strings.Contains(string(body), "claim_token") {
			t.Fatal("task credentials exposed")
		}
	}
	// Existing platform semantics intentionally aggregate all tenants.
	result, err := svc.Overview(context.Background(), 0, all, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inventory.SKUs != 2 || result.Inventory.Rows != 6 || result.Locations.Enabled != 9 {
		t.Fatalf("platform aggregate=%+v", result)
	}
}

type permissions map[string]bool

func (p permissions) HasPerm(_ context.Context, _ int64, perm string) bool { return p[perm] }

func TestOverviewDeniedSectionsDoNotQueryOrExposeBusinessTables(t *testing.T) {
	db, svc, now := fixture(t)
	// Removing a forbidden table proves the query is skipped, rather than merely hidden in JSON.
	if err := db.Migrator().DropTable(&inboundmodel.ReceiptOrder{}, &outboundmodel.ShipmentOrder{}, &taskmodel.Task{}); err != nil {
		t.Fatal(err)
	}
	result, err := svc.Overview(tenant.WithExactTenant(context.Background(), 99), 1, dto.Access{Inventory: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if result.Inbound != nil || result.Outbound != nil || result.TaskCounts != nil || result.Locations != nil || len(result.Warehouses) != 0 || result.Inventory.SKUs != 1 {
		t.Fatalf("forbidden sections=%+v", result)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(tenant.WithExactTenant(c.Request.Context(), 99))
	})
	handler.New(svc).RegisterRoutes(r.Group(""), permissions{})
	for _, query := range []string{"?warehouse_id=-1", "?warehouse_id=bad", "?warehouse_id=9223372036854775808"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/overview"+query, nil))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("bad filter accepted: %s", w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/overview", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "inventory\":{") {
		t.Fatalf("no permission response=%s", w.Body.String())
	}
	r = gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(tenant.WithExactTenant(c.Request.Context(), 99))
	})
	handler.New(svc).RegisterRoutes(r.Group(""), permissions{"wms:inventory": true})
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dashboard/overview?warehouse_id=1", nil))
	var body struct {
		Data dto.Overview `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || body.Data.Inventory == nil || body.Data.Inventory.SKUs != 1 || body.Data.Inbound != nil || body.Data.Access.Tasks {
		t.Fatalf("permission-filtered response=%s", w.Body.String())
	}
}
