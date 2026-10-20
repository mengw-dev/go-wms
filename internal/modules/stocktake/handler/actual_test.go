package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/stocktake/model"
	"gowms/internal/modules/stocktake/repository"
	"gowms/internal/modules/stocktake/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
	"gowms/internal/pkg/tx"
	"gowms/internal/testutil"
)

func TestActualQuantityMustBeExplicit(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn, &model.StocktakeOrder{}, &model.StocktakeDetail{})
	ctx := tenant.WithTenant(context.Background(), 11)
	order := model.StocktakeOrder{Base: modelbase.Base{ID: 1}, OrderNo: "ACTUAL-REQUIRED", Status: model.OrderDraft}
	detail := model.StocktakeDetail{Base: modelbase.Base{ID: 2}, OrderID: 1, BookQty: 10}
	if err := db.WithContext(ctx).Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&detail).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(), tx.New(db), nil, nil, config.LimitsConfig{})
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/orders/:id/actual", New(svc).recordActual)
	post := func(body string) int {
		t.Helper()
		req := httptest.NewRequest("POST", "/orders/1/actual", strings.NewReader(body)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var envelope struct {
			Code int `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Code
	}
	for _, body := range []string{`{"detail_id":"2"}`, `{"detail_id":"2","actual_qty":null}`, `{"detail_id":"2","actual_qty":-1}`, `{"detail_id":"2","actual_qty":2147483648}`} {
		if code := post(body); code != errcode.ParamError.Code {
			t.Fatalf("body=%s code=%d", body, code)
		}
		var current model.StocktakeDetail
		if err := db.WithContext(ctx).First(&current, detail.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.ActualQty != nil {
			t.Fatalf("invalid body changed actual_qty: %+v", current)
		}
		if err := svc.Approve(ctx, order.ID, "test", ""); !errors.Is(err, errcode.StocktakeNotFullyCounted) {
			t.Fatalf("approve after invalid count: %v", err)
		}
	}
	for _, qty := range []int{0, 8} {
		if code := post(fmt.Sprintf(`{"detail_id":"2","actual_qty":%d}`, qty)); code != 0 {
			t.Fatalf("qty=%d code=%d", qty, code)
		}
		var current model.StocktakeDetail
		if err := db.WithContext(ctx).First(&current, detail.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.ActualQty == nil || *current.ActualQty != qty {
			t.Fatalf("explicit qty=%d stored=%v", qty, current.ActualQty)
		}
	}
}
