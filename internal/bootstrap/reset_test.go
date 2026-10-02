package bootstrap

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

func resetTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true"
	}
	db := testutil.OpenIsolatedMySQL(t, dsn,
		&model.Warehouse{}, &model.Location{}, &model.SKU{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{},
		&taskmodel.Task{},
		&inboundmodel.ReceiptOrder{}, &inboundmodel.ReceiptOrderDetail{}, &inboundmodel.ImportTask{},
		&outboundmodel.ShipmentOrder{}, &outboundmodel.ShipmentOrderDetail{}, &outboundmodel.Allocation{},
		&stocktakemodel.StocktakeOrder{}, &stocktakemodel.StocktakeDetail{},
	)
	// 与生产一致：重置与种子都依赖租户回调做隔离/注入，测试库必须显式注册。
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		t.Fatalf("register tenant callbacks: %v", err)
	}
	return db
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

// tenantCounts 用于对比某租户在重置前后的各表行数。
type tenantCounts struct {
	warehouses     int64
	locations      int64
	skus           int64
	inventories    int64
	transfers      int64
	inboundOrders  int64
	outboundOrders int64
}

func tenantCountsOf(ctx context.Context, t *testing.T, db *gorm.DB) tenantCounts {
	t.Helper()
	count := func(entity any) int64 {
		var n int64
		if err := db.WithContext(ctx).Model(entity).Count(&n).Error; err != nil {
			t.Fatalf("count %T: %v", entity, err)
		}
		return n
	}
	return tenantCounts{
		warehouses:     count(&model.Warehouse{}),
		locations:      count(&model.Location{}),
		skus:           count(&model.SKU{}),
		inventories:    count(&invmodel.Inventory{}),
		transfers:      count(&invmodel.InventoryTrans{}),
		inboundOrders:  count(&inboundmodel.ReceiptOrder{}),
		outboundOrders: count(&outboundmodel.ShipmentOrder{}),
	}
}

// seedTenantSentinel 在指定租户写入一组可识别的业务数据（仓库/库位/货品/库存/流水/入库单）。
func seedTenantSentinel(ctx context.Context, t *testing.T, db *gorm.DB, code string) int64 {
	t.Helper()
	warehouse := model.Warehouse{Code: code, Name: code + " 仓库", Status: 1}
	if err := db.WithContext(ctx).Create(&warehouse).Error; err != nil {
		t.Fatalf("create warehouse: %v", err)
	}
	location := model.Location{Code: code + "-L01", Zone: "Z01", WarehouseID: warehouse.ID, Status: model.LocationStatusIdle}
	if err := db.WithContext(ctx).Create(&location).Error; err != nil {
		t.Fatalf("create location: %v", err)
	}
	sku := model.SKU{Code: code + "-SKU", Barcode: code + "-BAR", Name: code + " 货品", Status: 1}
	if err := db.WithContext(ctx).Create(&sku).Error; err != nil {
		t.Fatalf("create sku: %v", err)
	}
	inventory := invmodel.Inventory{
		WarehouseID: warehouse.ID, LocationID: location.ID, SKUID: sku.ID,
		BatchNo: code + "-B1", StockQuantity: 7, AvailableQty: 7, AllocatedQty: 0,
		StockInTime: time.Now(),
	}
	if err := db.WithContext(ctx).Create(&inventory).Error; err != nil {
		t.Fatalf("create inventory: %v", err)
	}
	trans := invmodel.InventoryTrans{
		InventoryID: inventory.ID, TransType: invmodel.TransReceive,
		QuantityChange: 7, BeforeQuantity: 0, AfterQuantity: 7,
		AvailableBefore: 0, AvailableAfter: 7, OrderNo: code + "-RK", Operator: "test",
	}
	if err := db.WithContext(ctx).Create(&trans).Error; err != nil {
		t.Fatalf("create inventory trans: %v", err)
	}
	order := inboundmodel.ReceiptOrder{
		OrderNo: code + "-ORDER", WarehouseID: warehouse.ID, Status: inboundmodel.OrderCompleted,
		Source: "MANUAL", ExpectedQty: 7, ReceivedQty: 7, CreatedBy: "test",
	}
	if err := db.WithContext(ctx).Create(&order).Error; err != nil {
		t.Fatalf("create inbound order: %v", err)
	}
	return warehouse.ID
}

// TestResetDemoDataOnlyAffectsTargetTenant 双租户隔离：
// 重置一个租户后，它自己的数据被清空并替换为演示种子，另一个租户逐表不变。
func TestResetDemoDataOnlyAffectsTargetTenant(t *testing.T) {
	db := resetTestDB(t)
	const (
		resetTenantID = int64(100)
		keepTenantID  = int64(200)
	)
	ctxReset := tenant.WithTenant(context.Background(), resetTenantID)
	ctxKeep := tenant.WithTenant(context.Background(), keepTenantID)

	resetWarehouseID := seedTenantSentinel(ctxReset, t, db, "A-SENTINEL")
	keepWarehouseID := seedTenantSentinel(ctxKeep, t, db, "B-SENTINEL")
	if resetWarehouseID == 0 || keepWarehouseID == 0 {
		t.Fatal("sentinel warehouses should be created")
	}
	before := tenantCountsOf(ctxKeep, t, db)

	if err := ResetDemoData(context.Background(), db, resetTenantID); err != nil {
		t.Fatalf("ResetDemoData: %v", err)
	}

	// 被重置的租户：原有业务数据不再存在，且已替换为演示种子（不是只清空）。
	var resetOldWarehouses int64
	if err := db.WithContext(ctxReset).Model(&model.Warehouse{}).Where("code = ?", "A-SENTINEL").Count(&resetOldWarehouses).Error; err != nil {
		t.Fatal(err)
	}
	if resetOldWarehouses != 0 {
		t.Fatalf("reset tenant still has old warehouse rows: %d", resetOldWarehouses)
	}
	var demoWarehouses int64
	if err := db.WithContext(ctxReset).Model(&model.Warehouse{}).Where("code = ?", "WH01").Count(&demoWarehouses).Error; err != nil {
		t.Fatal(err)
	}
	if demoWarehouses != 1 {
		t.Fatalf("reset tenant should be re-seeded with demo data, WH01 count=%d", demoWarehouses)
	}
	if seeded := tenantCountsOf(ctxReset, t, db); seeded.skus != 5 || seeded.inventories != 4 {
		t.Fatalf("reset tenant demo seed mismatch: skus=%d inventories=%d want 5/4", seeded.skus, seeded.inventories)
	}

	// 未重置的租户：哨兵数据完整，且各表行数与重置前完全一致。
	var keepWarehouse model.Warehouse
	if err := db.WithContext(ctxKeep).Where("id = ?", keepWarehouseID).First(&keepWarehouse).Error; err != nil {
		t.Fatalf("unrelated tenant warehouse lost: %v", err)
	}
	if keepWarehouse.Code != "B-SENTINEL" || keepWarehouse.Status != 1 {
		t.Fatalf("unrelated tenant warehouse changed: %+v", keepWarehouse)
	}
	if after := tenantCountsOf(ctxKeep, t, db); after != before {
		t.Fatalf("unrelated tenant changed: before=%+v after=%+v", before, after)
	}
}
