package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"gowms/internal/modules/basic/model"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	outboundmodel "gowms/internal/modules/outbound/model"
	stocktakemodel "gowms/internal/modules/stocktake/model"
	sysmodel "gowms/internal/modules/system/model"
	taskmodel "gowms/internal/modules/task/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/log"
)

// InitDB 初始化 GORM MySQL 连接。
func InitDB(cfg *config.Config) (*gorm.DB, error) {
	logLevel := logger.Warn
	if cfg.Server.Mode == "debug" {
		logLevel = logger.Info
	}
	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN), &gorm.Config{
		Logger: logger.Default.LogMode(logLevel),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 表名单数，与 TableName() 一致
		},
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MySQL.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MySQL.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// InitRedis 初始化 Redis（不可用时仅记录告警，业务自动降级）。
func InitRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		slog.Warn("redis unavailable, order generator will fallback to local mode", "err", err)
	}
	return rdb
}

// Migrate 仅供开发和测试使用：AutoMigrate + 种子数据。
func Migrate(db *gorm.DB) error {
	if err := AutoMigrate(db); err != nil {
		return err
	}
	return Seed(db)
}

// AutoMigrate 自动迁移表结构 + CHECK 约束。生产环境应使用 cmd/migrate。
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&sysmodel.SysUser{}, &sysmodel.SysRole{}, &sysmodel.SysUserRole{}, &sysmodel.SysOperLog{},
		&model.Warehouse{}, &model.Location{}, &model.SKU{},
		&invmodel.Inventory{}, &invmodel.InventoryTrans{},
		&taskmodel.Task{},
		&inboundmodel.ReceiptOrder{}, &inboundmodel.ReceiptOrderDetail{}, &inboundmodel.ImportTask{},
		&outboundmodel.ShipmentOrder{}, &outboundmodel.ShipmentOrderDetail{}, &outboundmodel.Allocation{},
		&stocktakemodel.StocktakeOrder{}, &stocktakemodel.StocktakeDetail{},
	); err != nil {
		return err
	}
	// CHECK 约束（MySQL 8.0.16+ 强制执行）；已存在时报 1061 duplicate，属预期可忽略
	if err := db.Exec("ALTER TABLE wms_inventory ADD CONSTRAINT chk_inv_non_negative CHECK (available_quantity >= 0 AND stock_quantity >= 0)").Error; err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate") &&
			!strings.Contains(err.Error(), "1061") {
			log.L().Warn("add inventory check constraint failed", "err", err)
		}
	}
	return nil
}

// Seed 写入内置账号与演示基础数据（幂等：各类数据已存在时分别跳过）。
func Seed(db *gorm.DB) error {
	if err := seedAdmin(db); err != nil {
		return err
	}
	return seedDemoData(db)
}

// seedAdmin 内置管理员与角色：admin / admin123。
func seedAdmin(db *gorm.DB) error {
	var n int64

	if err := db.Model(&sysmodel.SysUser{}).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	admin := &sysmodel.SysUser{Username: "admin", PasswordHash: string(hash), Nickname: "管理员", Status: 1}
	role := &sysmodel.SysRole{Name: "admin", Perms: "*", Remark: "内置超级管理员"}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(admin).Error; err != nil {
			return err
		}
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		return tx.Create(&sysmodel.SysUserRole{UserID: admin.ID, RoleID: role.ID}).Error
	})
}

// demoPlacement 描述一条演示库存：仓库序号-库位编码-SKU序号-批次-数量-入库时间。
type demoPlacement struct {
	warehouse int
	location  string
	sku       int
	batch     string
	qty       int
	stockIn   time.Time
}

// seedDemoData 写入演示用基础资料与库存（仓库、库位、SKU、库存、入库流水）。
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
		{Code: "WH02", Name: "华南二号仓", Remark: "演示数据：广州区域仓", Status: 1},
	}

	zones := []string{"A01", "B01"}
	var locations []model.Location
	for range warehouses {
		for _, zone := range zones {
			for row := 1; row <= 2; row++ {
				for col := 1; col <= 3; col++ {
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
		{Code: "SKU000006", Barcode: "6901234500066", Name: "维达超韧抽纸", Spec: "3层120抽×24包", Unit: "箱", Status: 1},
		{Code: "SKU000007", Barcode: "6901234500073", Name: "蓝月亮深层洁净洗衣液", Spec: "3kg", Unit: "瓶", Status: 1},
		{Code: "SKU000008", Barcode: "6901234500080", Name: "南孚5号碱性电池", Spec: "1.5V×40粒", Unit: "盒", Status: 1},
		{Code: "SKU000009", Barcode: "6901234500097", Name: "苏泊尔电热水壶", Spec: "1.5L 1800W", Unit: "台", Status: 1},
		{Code: "SKU000010", Barcode: "6901234500103", Name: "3M防护口罩", Spec: "KN95×50只", Unit: "盒", Status: 1},
	}

	date := func(s string) time.Time {
		t, _ := time.ParseInLocation("2006-01-02", s, time.Local)
		return t
	}
	placements := []demoPlacement{
		{warehouse: 0, location: "A01-01-01", sku: 0, batch: "B20260801", qty: 1200, stockIn: date("2026-08-01")},
		{warehouse: 0, location: "A01-01-02", sku: 1, batch: "B20260805", qty: 600, stockIn: date("2026-08-05")},
		{warehouse: 0, location: "A01-01-03", sku: 2, batch: "B20260720", qty: 480, stockIn: date("2026-07-20")},
		{warehouse: 0, location: "A01-02-01", sku: 5, batch: "B20260810", qty: 300, stockIn: date("2026-08-10")},
		{warehouse: 0, location: "A01-02-02", sku: 6, batch: "B20260615", qty: 150, stockIn: date("2026-06-15")},
		{warehouse: 0, location: "A01-02-03", sku: 0, batch: "B20260901", qty: 600, stockIn: date("2026-09-01")},
		{warehouse: 0, location: "B01-01-01", sku: 8, batch: "B20260501", qty: 60, stockIn: date("2026-05-01")},
		{warehouse: 0, location: "B01-01-02", sku: 7, batch: "B20260820", qty: 800, stockIn: date("2026-08-20")},
		{warehouse: 0, location: "B01-02-01", sku: 3, batch: "B20260710", qty: 240, stockIn: date("2026-07-10")},
		{warehouse: 0, location: "B01-02-02", sku: 9, batch: "B20260905", qty: 200, stockIn: date("2026-09-05")},
		{warehouse: 1, location: "A01-01-01", sku: 0, batch: "B20260812", qty: 400, stockIn: date("2026-08-12")},
		{warehouse: 1, location: "A01-01-02", sku: 4, batch: "B20260818", qty: 500, stockIn: date("2026-08-18")},
		{warehouse: 1, location: "A01-02-01", sku: 5, batch: "B20260902", qty: 120, stockIn: date("2026-09-02")},
		{warehouse: 1, location: "B01-01-01", sku: 6, batch: "B20260701", qty: 80, stockIn: date("2026-07-01")},
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&warehouses).Error; err != nil {
			return err
		}
		locIndex := make(map[string]int64)
		for whIdx := range warehouses {
			for i := range locations {
				if i/12 != whIdx {
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

			orderNo := fmt.Sprintf("RK%s%06d", p.stockIn.Format("20060102"), i+1)
			trans := invmodel.InventoryTrans{
				InventoryID:     inventory.ID,
				TransType:       invmodel.TransReceive,
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
