package bootstrap

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/basic/model"
	invmodel "gowms/internal/modules/inventory/model"
)

// demoPlacement 描述一条演示库存：仓库序号-库位编码-SKU序号-批次-数量-入库时间。
type demoPlacement struct {
	warehouse int
	location  string
	sku       int
	batch     string
	qty       int
	stockIn   time.Time
}

// seedDemoData 只写基础资料及明确标记来源的期初库存，不伪造已完成业务单据。
// 仅当系统中尚不存在任何仓库时执行，避免覆盖使用者自行创建的业务数据。
func seedDemoData(db *gorm.DB) error {
	var warehouseCount int64
	if err := db.Model(&model.Warehouse{}).Count(&warehouseCount).Error; err != nil {
		return err
	}
	if warehouseCount > 0 {
		return nil
	}

	warehouses := []model.Warehouse{
		{Code: "WH01", Name: "华东一号仓", Remark: "演示数据：上海中心仓", Status: 1},
	}

	zones := []string{"A01"}
	var locations []model.Location
	for range warehouses {
		for _, zone := range zones {
			for row := 1; row <= 2; row++ {
				for col := 1; col <= 2; col++ {
					locations = append(locations, model.Location{
						Code:   fmt.Sprintf("%s-%02d-%02d", zone, row, col),
						Zone:   zone,
						Status: model.LocationStatusIdle,
					})
				}
			}
		}
	}

	skus := []model.SKU{
		{Code: "SKU000001", Barcode: "6901234500011", Name: "农夫山泉饮用天然水", Spec: "550ml×24瓶", Unit: "箱", Status: 1},
		{Code: "SKU000002", Barcode: "6901234500028", Name: "可口可乐汽水", Spec: "330ml×24罐", Unit: "箱", Status: 1},
		{Code: "SKU000003", Barcode: "6901234500035", Name: "康师傅红烧牛肉面", Spec: "105g×12桶", Unit: "箱", Status: 1},
		{Code: "SKU000004", Barcode: "6901234500042", Name: "旺旺雪饼", Spec: "540g", Unit: "袋", Status: 1},
		{Code: "SKU000005", Barcode: "6901234500059", Name: "双汇王中王火腿肠", Spec: "60g×40支", Unit: "箱", Status: 1},
	}

	date := func(s string) time.Time {
		t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
		return t
	}
	placements := []demoPlacement{
		// SKU000001 保留同 SKU 的两个批次，供自动出库真实展示 FIFO 跨批次分配。
		{warehouse: 0, location: "A01-01-01", sku: 0, batch: "B20260901", qty: 30, stockIn: date("2026-09-01")},
		{warehouse: 0, location: "A01-01-01", sku: 0, batch: "B20260905", qty: 70, stockIn: date("2026-09-05")},
		{warehouse: 0, location: "A01-01-02", sku: 1, batch: "B20260905", qty: 50, stockIn: date("2026-09-05")},
		{warehouse: 0, location: "A01-02-01", sku: 2, batch: "B20260910", qty: 80, stockIn: date("2026-09-10")},
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&warehouses).Error; err != nil {
			return err
		}
		locIndex := make(map[string]int64)
		for whIdx := range warehouses {
			for i := range locations {
				if i/4 != whIdx {
					continue
				}
				loc := locations[i]
				loc.WarehouseID = warehouses[whIdx].ID
				if err := tx.Create(&loc).Error; err != nil {
					return err
				}
				locIndex[fmt.Sprintf("%d|%s", whIdx, loc.Code)] = loc.ID
			}
		}
		if err := tx.Create(&skus).Error; err != nil {
			return err
		}

		occupied := make(map[int64]struct{})
		for i, p := range placements {
			locationID := locIndex[fmt.Sprintf("%d|%s", p.warehouse, p.location)]
			inventory := invmodel.Inventory{
				WarehouseID:   warehouses[p.warehouse].ID,
				LocationID:    locationID,
				SKUID:         skus[p.sku].ID,
				BatchNo:       p.batch,
				StockQuantity: p.qty,
				AvailableQty:  p.qty,
				AllocatedQty:  0,
				StockInTime:   p.stockIn,
			}
			if err := tx.Create(&inventory).Error; err != nil {
				return err
			}
			occupied[locationID] = struct{}{}

			orderNo := fmt.Sprintf("SEED-%s-%06d", p.stockIn.Format("20060102"), i+1)
			trans := invmodel.InventoryTrans{
				InventoryID:     inventory.ID,
				TransType:       invmodel.TransOpening,
				QuantityChange:  p.qty,
				BeforeQuantity:  0,
				AfterQuantity:   p.qty,
				AvailableBefore: 0,
				AvailableAfter:  p.qty,
				OrderNo:         orderNo,
				Operator:        "system-seed",
				CreatedAt:       p.stockIn,
			}
			if err := tx.Create(&trans).Error; err != nil {
				return err
			}
		}

		occupiedIDs := make([]int64, 0, len(occupied))
		for id := range occupied {
			occupiedIDs = append(occupiedIDs, id)
		}
		return tx.Model(&model.Location{}).Where("id IN ?", occupiedIDs).
			Update("status", model.LocationStatusOccupied).Error
	})
}
