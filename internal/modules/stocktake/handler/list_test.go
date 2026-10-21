package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/stocktake/model"
	"gowms/internal/modules/stocktake/repository"
	"gowms/internal/modules/stocktake/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

// Exercise query binding, the service and real MySQL together: a UI keyword
// silently dropped at any layer must not return an unfiltered list or total.
func TestListStocktakeOrdersSearch(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.StocktakeOrder{})
	ctx := tenant.WithTenant(context.Background(), 11)
	for _, row := range []struct {
		id, tenantID, warehouse int64
		no                      string
		status                  model.OrderStatus
		deleted                 bool
	}{
		{1, 11, 21, "PD-ALPHA-001", model.OrderDraft, false},
		{2, 11, 21, "PD-ALPHA-002", model.OrderDraft, false},
		{3, 11, 21, "PD-BETA-003", model.OrderDraft, false},
		{4, 11, 21, "PD-ALPHA-004", model.OrderCompleted, false},
		{5, 11, 22, "PD-ALPHA-005", model.OrderDraft, false},
		{6, 22, 21, "PD-ALPHA-001", model.OrderDraft, false},
		{7, 11, 21, "PD-ALPHA-DELETED", model.OrderDraft, true},
		{8, 11, 21, `PD-LIT%_\-008`, model.OrderDraft, false},
		{9, 0, 21, "PD-PLATFORM-009", model.OrderDraft, false},
		{10, 11, 21, "PD-NORMAL-010", model.OrderDraft, false},
	} {
		rowCtx := tenant.WithExactTenant(ctx, row.tenantID)
		order := &model.StocktakeOrder{Base: modelbase.Base{ID: row.id}, WarehouseID: row.warehouse, OrderNo: row.no, Status: row.status}
		if err := db.WithContext(rowCtx).Create(order).Error; err != nil {
			t.Fatal(err)
		}
		if row.deleted {
			if err := db.WithContext(rowCtx).Delete(order).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	svc := service.New(repository.New(), tx.New(db), nil, nil, config.LimitsConfig{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/orders", New(svc).list)
	for _, tc := range []struct {
		name, query string
		ctx         context.Context
		ids         string
		total       int64
	}{
		{"partial", "keyword=ALPHA", ctx, "5,4,2,1", 4},
		{"combined_first_page", "keyword=ALPHA&warehouse_id=21&status=DRAFT&page_size=1", ctx, "2", 2},
		{"combined_second_page", "keyword=ALPHA&warehouse_id=21&status=DRAFT&page_size=1&page=2", ctx, "1", 2},
		{"past_last_page", "keyword=ALPHA&warehouse_id=21&status=DRAFT&page_size=1&page=3", ctx, "", 2},
		{"empty", "keyword=NOT-FOUND", ctx, "", 0},
		{"literal_percent", "keyword=%25", ctx, "8", 1},
		{"literal_underscore", "keyword=_", ctx, "8", 1},
		{"literal_backslash", "keyword=%5C", ctx, "8", 1},
		{"sql_text", "keyword=%27%20OR%201%3D1%20--", ctx, "", 0},
		{"no_keyword", "warehouse_id=21&status=DRAFT", ctx, "10,8,3,2,1", 5},
		{"other_tenant", "keyword=ALPHA", tenant.WithTenant(ctx, 22), "6", 1},
		{"exact_platform", "keyword=PLATFORM", tenant.WithExactTenant(ctx, 0), "9", 1},
		{"exact_platform_cannot_see_tenant", "keyword=ALPHA", tenant.WithExactTenant(ctx, 0), "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/orders?"+tc.query, nil).WithContext(tc.ctx)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
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
			if recorder.Code != http.StatusOK || body.Code != 0 || body.Data.Total != tc.total || strings.Join(ids, ",") != tc.ids {
				t.Fatalf("status=%d body=%s want ids=%s total=%d", recorder.Code, recorder.Body.String(), tc.ids, tc.total)
			}
		})
	}
}
