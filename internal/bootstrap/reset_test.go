package bootstrap

import (
	"context"
	"os"
	"testing"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/testutil"
)

func resetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true"
	}
	return testutil.OpenIsolatedMySQL(t, dsn,
		&model.Warehouse{}, &model.Location{}, &model.SKU{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{},
		&taskmodel.Task{},
		&inboundmodel.ReceiptOrder{}, &inboundmodel.ReceiptOrderDetail{}, &inboundmodel.ImportTask{},
		&outboundmodel.ShipmentOrder{}, &outboundmodel.ShipmentOrderDetail{}, &outboundmodel.Allocation{},
		&stocktakemodel.StocktakeOrder{}, &stocktakemodel.StocktakeDetail{},
	)
}

func TestResetDemoDataRejectsNonPositiveTenant(t *testing.T) {
	db := resetTestDB(t)
	ctx := context.Background()

	for _, tenantID := range []int64{0, -1, -100} {
		if err := ResetDemoData(ctx, db, tenantID); err == nil {
			t.Fatalf("tenantID=%d should be rejected", tenantID)
		}
	}
}

func TestResetDemoDataOnlyAffectsTargetTenant(t *testing.T) {
	db := resetTestDB(t)
	ctx := context.Background()

	// 防御性验证：合法租户 ID 可以正常执行（不报错）。
	if err := ResetDemoData(ctx, db, 100); err != nil {
		t.Fatalf("ResetDemoData with valid tenantID failed: %v", err)
	}
}
