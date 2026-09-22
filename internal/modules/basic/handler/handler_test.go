package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/basic/dto"
)

func TestBasicListQueriesBindZeroStatusAndLocationFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	warehouseCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	warehouseCtx.Request = httptest.NewRequest("GET", "/basic/warehouses?status=0", nil)
	var warehouseQuery dto.WarehouseQuery
	if err := warehouseCtx.ShouldBindQuery(&warehouseQuery); err != nil {
		t.Fatalf("bind warehouse query: %v", err)
	}
	if warehouseQuery.Status == nil || *warehouseQuery.Status != 0 {
		t.Fatalf("warehouse status=%v want pointer to 0", warehouseQuery.Status)
	}

	locationCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	locationCtx.Request = httptest.NewRequest("GET", "/basic/locations?warehouse_id=7&zone=A01&status=0&keyword=A01", nil)
	var locationQuery dto.LocationQuery
	if err := locationCtx.ShouldBindQuery(&locationQuery); err != nil {
		t.Fatalf("bind location query: %v", err)
	}
	if locationQuery.WarehouseID != 7 || locationQuery.Zone != "A01" || locationQuery.Keyword != "A01" {
		t.Fatalf("location query=%+v", locationQuery)
	}
	if locationQuery.Status == nil || *locationQuery.Status != 0 {
		t.Fatalf("location status=%v want pointer to 0", locationQuery.Status)
	}
}
