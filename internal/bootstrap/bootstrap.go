package bootstrap

import (
	"context"
	"errors"
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
func Migrate(db *gorm.DB, cfg *config.Config) error {
	if err := AutoMigrate(db); err != nil {
		return err
	}
	if err := Seed(db); err != nil {
		return err
	}
	return SeedDemoAccount(db, cfg)
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

// seedDemoData 写入演示用基础资料与库存（仓库、库位、SKU、库存、入库流水、演示单据）。
// 仅当系统中尚不存在任何仓库时执行，避免覆盖使用者自行创建的业务数据。
func seedDemoData(db *gorm.DB) error {
	var warehouseCount int64
	if err := db.Model(&model.Warehouse{}).Count(&warehouseCount).Error; err != nil {
		return err
	}
	if warehouseCount > 0 {
		return nil
	}

	// 幂等：已存在任何入库/出库单则跳过单据种入
	var existingInbound int64
	var existingOutbound int64
	if err := db.Model(&inboundmodel.ReceiptOrder{}).Count(&existingInbound).Error; err != nil {
		log.L().Warn("count existing inbound orders failed, skip demo order seeding", "err", err)
	}
	if err := db.Model(&outboundmodel.ShipmentOrder{}).Count(&existingOutbound).Error; err != nil {
		log.L().Warn("count existing outbound orders failed, skip demo order seeding", "err", err)
	}
	seedDemoOrders := existingInbound == 0 && existingOutbound == 0

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
		{warehouse: 0, location: "A01-01-01", sku: 0, batch: "B20260901", qty: 100, stockIn: date("2026-09-01")},
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
		if err := tx.Model(&model.Location{}).Where("id IN ?", occupiedIDs).
			Update("status", model.LocationStatusOccupied).Error; err != nil {
			return err
		}

		// ---- 演示单据（仅在系统里完全没单据时种入） ----
		if !seedDemoOrders {
			return nil
		}

		inboundOrders := []inboundmodel.ReceiptOrder{
			{
				OrderNo: "RK20260901000001", WarehouseID: warehouses[0].ID,
				Status: inboundmodel.OrderCompleted, Source: "MANUAL",
				Remark: "演示数据：SKU000001 农夫山泉", ExpectedQty: 100, ReceivedQty: 100, CreatedBy: "system-seed",
			},
			{
				OrderNo: "RK20260905000001", WarehouseID: warehouses[0].ID,
				Status: inboundmodel.OrderCompleted, Source: "MANUAL",
				Remark: "演示数据：SKU000002 可口可乐", ExpectedQty: 50, ReceivedQty: 50, CreatedBy: "system-seed",
			},
		}
		for _, o := range inboundOrders {
			if err := tx.Create(&o).Error; err != nil {
				return err
			}
		}

		outboundOrder := outboundmodel.ShipmentOrder{
			OrderNo: "CK20260915000001", BizOrderNo: "CUST20260915001", WarehouseID: warehouses[0].ID,
			Status: outboundmodel.OrderShipped, Remark: "演示数据：客户订单 CUST20260915001",
			ExpectedQty: 30, PickedQty: 30, CreatedBy: "system-seed",
		}
		if err := tx.Create(&outboundOrder).Error; err != nil {
			return err
		}

		return nil
	})
}

// SeedDemoAccount 幂等创建公开演示账号。只有 WMS_DEMO_ENABLED=true 时才创建。
func SeedDemoAccount(db *gorm.DB, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Demo.Enabled {
		// 关闭演示模式时同步禁用既有演示账号，避免旧账号绕过 DemoSession 中间件。
		return db.Model(&sysmodel.SysUser{}).Where("username = ?", cfg.Demo.Username).
			Update("status", 0).Error
	}
	const demoPerms = "wms:basic,wms:inventory,wms:task," +
		"wms:inbound:view,wms:inbound:create,wms:inbound:submit,wms:inbound:approve,wms:inbound:cancel,wms:inbound:receive,wms:inbound:putaway," +
		"wms:outbound:view,wms:outbound:create,wms:outbound:submit,wms:outbound:approve,wms:outbound:cancel,wms:outbound:pick," +
		"wms:stocktake:view,wms:stocktake:create,wms:stocktake:stocktake,wms:stocktake:approve,wms:stocktake:cancel,wms:demo"

	var role sysmodel.SysRole
	err := db.Where("name = ?", "demo").First(&role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		role = sysmodel.SysRole{Name: "demo", Perms: demoPerms, Remark: "公开业务体验账号，仅允许业务操作"}
		if err := db.Create(&role).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := db.Model(&role).Updates(map[string]any{
		"perms": demoPerms, "remark": "公开业务体验账号，仅允许业务操作",
	}).Error; err != nil {
		return err
	}

	// 每个实例只保留当前配置的演示账号，避免改用户名后旧演示账号继续绕过单会话锁。
	var linkedUserIDs []int64
	if err := db.Model(&sysmodel.SysUserRole{}).Where("role_id = ?", role.ID).
		Pluck("user_id", &linkedUserIDs).Error; err != nil {
		return err
	}
	if len(linkedUserIDs) > 0 {
		if err := db.Model(&sysmodel.SysUser{}).
			Where("id IN ? AND username <> ?", linkedUserIDs, cfg.Demo.Username).
			Update("status", 0).Error; err != nil {
			return err
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Demo.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var user sysmodel.SysUser
	err = db.Where("username = ?", cfg.Demo.Username).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		user = sysmodel.SysUser{
			Username: cfg.Demo.Username, PasswordHash: string(hash), Nickname: "业务体验账号", Status: 1,
		}
		if err := db.Create(&user).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if err := db.Model(&user).Updates(map[string]any{
		"password_hash": string(hash), "nickname": "业务体验账号", "status": 1,
	}).Error; err != nil {
		return err
	}

	var link sysmodel.SysUserRole
	err = db.Where("user_id = ? AND role_id = ?", user.ID, role.ID).First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.Create(&sysmodel.SysUserRole{UserID: user.ID, RoleID: role.ID}).Error
	}
	return err
}

// ResetDemoData 硬删除所有演示业务数据并重新写入默认演示数据。
// 用户、角色、迁移记录和操作日志不会删除。
func ResetDemoData(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		models := []any{
			&taskmodel.Task{},
			&outboundmodel.Allocation{},
			&outboundmodel.ShipmentOrderDetail{},
			&outboundmodel.ShipmentOrder{},
			&inboundmodel.ReceiptOrderDetail{},
			&inboundmodel.ReceiptOrder{},
			&inboundmodel.ImportTask{},
			&stocktakemodel.StocktakeDetail{},
			&stocktakemodel.StocktakeOrder{},
			&invmodel.InventoryTrans{},
			&invmodel.Inventory{},
			&model.Location{},
			&model.SKU{},
			&model.Warehouse{},
		}
		for _, item := range models {
			if err := tx.Unscoped().Where("1 = 1").Delete(item).Error; err != nil {
				return err
			}
		}
		return seedDemoData(tx)
	})
}
