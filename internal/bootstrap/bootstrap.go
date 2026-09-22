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
	"gowms/internal/pkg/tenant"
)

// InitDB 初始化 GORM MySQL 连接，并注册多租户全局回调。
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
	// 多租户全局隔离：ctx 有租户时自动注入 WHERE tenant_id = ?；
	// 迁移/种子等 context.Background() 场景天然旁路（tenant_id=0）。
	if err := tenant.RegisterGORMCallbacks(db); err != nil {
		return nil, fmt.Errorf("register tenant callbacks: %w", err)
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

// InitRedis 初始化 Redis。缓存和单号可以降级；会话校验等安全操作仍需拒绝故障请求。
func InitRedis(cfg *config.Config) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB,
		ContextTimeoutEnabled: true,
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
	if err := SeedDemoAccounts(db, cfg); err != nil {
		return err
	}
	return SeedPersonalAccounts(db, cfg)
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
	return db.Transaction(func(tx *gorm.DB) error {
		var admin sysmodel.SysUser
		err := tx.Where("tenant_id = ? AND username = ?", 0, "admin").First(&admin).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			hash, hashErr := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
			if hashErr != nil {
				return hashErr
			}
			admin = sysmodel.SysUser{TenantID: 0, Username: "admin", PasswordHash: string(hash), Nickname: "管理员", Status: 1}
			if err := tx.Create(&admin).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var role sysmodel.SysRole
		err = tx.Where("tenant_id = ? AND name = ?", 0, "admin").First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = sysmodel.SysRole{TenantID: 0, Name: "admin", Perms: "*", Remark: "内置超级管理员"}
			if err := tx.Create(&role).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var link sysmodel.SysUserRole
		err = tx.Where("tenant_id = ? AND user_id = ? AND role_id = ?", 0, admin.ID, role.ID).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(&sysmodel.SysUserRole{TenantID: 0, UserID: admin.ID, RoleID: role.ID}).Error
		}
		return err
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

// demoPerms 演示账号权限集合（只读 + 业务操作 + 演示中心，无系统管理权限）。
const demoPerms = "wms:basic,wms:inventory,wms:task," +
	"wms:inbound:view,wms:inbound:create,wms:inbound:submit,wms:inbound:approve,wms:inbound:cancel,wms:inbound:receive,wms:inbound:putaway," +
	"wms:outbound:view,wms:outbound:create,wms:outbound:submit,wms:outbound:approve,wms:outbound:cancel,wms:outbound:pick," +
	"wms:stocktake:view,wms:stocktake:create,wms:stocktake:stocktake,wms:stocktake:approve,wms:stocktake:cancel,wms:demo"

// SeedDemoAccounts 幂等创建多演示账号（demo1..demoN）：每个账号独占一个租户（10001+）
// 与一份该租户内的演示角色（角色查询走租户过滤，必须与用户同租户），数据互不影响。
// 同时禁用不在当前名单内的历史演示账号（数量收缩后多余的账号、旧的单 demo 账号），
// 避免旧账号绕过按租户的会话锁。只有 WMS_DEMO_ENABLED=true 时才创建。
func SeedDemoAccounts(db *gorm.DB, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Demo.Enabled || cfg.Demo.Instances <= 0 {
		// 关闭演示模式：禁用全部历史演示账号，避免旧账号绕过 DemoSession 中间件。
		return db.Model(&sysmodel.SysUser{}).
			Where("tenant_id IN ? AND (username = ? OR username REGEXP ?)", managedDemoTenantIDs(), "demo", "^demo[0-9]+$").
			Update("status", 0).Error
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Demo.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	current := make([]string, 0, cfg.Demo.Instances)
	for i := 1; i <= cfg.Demo.Instances; i++ {
		username := cfg.Demo.AccountUsername(i)
		tenantID := cfg.Demo.AccountTenantID(i)
		nickname := fmt.Sprintf("演示访客%d", i)
		current = append(current, username)

		// 演示角色（每租户一份，幂等：存在则同步权限）
		var role sysmodel.SysRole
		err := db.Where("tenant_id = ? AND name = ?", tenantID, "demo").First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = sysmodel.SysRole{TenantID: tenantID, Name: "demo", Perms: demoPerms, Remark: "公开业务体验账号，仅允许业务操作"}
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

		// 演示用户（幂等：存在则同步密码/昵称并启用）
		var user sysmodel.SysUser
		err = db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = sysmodel.SysUser{
				TenantID: tenantID, Username: username, PasswordHash: string(hash),
				Nickname: nickname, Status: 1,
			}
			if err := db.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&user).Updates(map[string]any{
			"password_hash": string(hash), "nickname": nickname, "status": 1,
		}).Error; err != nil {
			return err
		}

		// 用户-角色关联（幂等）
		var link sysmodel.SysUserRole
		err = db.Where("user_id = ? AND role_id = ?", user.ID, role.ID).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&sysmodel.SysUserRole{TenantID: tenantID, UserID: user.ID, RoleID: role.ID}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	// 禁用不在当前名单内的历史演示账号（收缩数量后多余的 demoN+1.. 与旧 demo）
	return db.Model(&sysmodel.SysUser{}).
		Where("tenant_id IN ? AND (username = ? OR username REGEXP ?) AND username NOT IN ?",
			managedDemoTenantIDs(), "demo", "^demo[0-9]+$", current).
		Update("status", 0).Error
}

func managedDemoTenantIDs() []int64 {
	ids := make([]int64, 0, 99)
	var cfg config.DemoConfig
	for i := 1; i <= 99; i++ {
		ids = append(ids, cfg.AccountTenantID(i))
	}
	return ids
}

// personalPerms 持久体验账号权限集合：与演示账号一致，但【不含 wms:demo】。
// 原因：带 wms:demo 的账号登录后前端会领取演示会话，而那会重置本租户数据；
// 持久账号的数据必须长期保留，因此既不加会话锁、也不参与演示重置。
const personalPerms = "wms:basic,wms:inventory,wms:task," +
	"wms:inbound:view,wms:inbound:create,wms:inbound:submit,wms:inbound:approve,wms:inbound:cancel,wms:inbound:receive,wms:inbound:putaway," +
	"wms:outbound:view,wms:outbound:create,wms:outbound:submit,wms:outbound:approve,wms:outbound:cancel,wms:outbound:pick," +
	"wms:stocktake:view,wms:stocktake:create,wms:stocktake:stocktake,wms:stocktake:approve,wms:stocktake:cancel"

// SeedPersonalAccounts 幂等创建持久体验账号（user1..userN）：每个账号独占一个租户（20001+），
// 数据互不影响；与演示账号的区别是数据长期保留（不重置、无会话锁），
// 且登录页由访客自行挑选账号（见 demo 模块的 /personal/* 接口）。
// 首次创建租户时种入一份初始业务数据，之后任何路径都不会再重置该租户。
// 只有 WMS_PERSONAL_ENABLED=true 时才创建；关闭时禁用历史个人账号。
func SeedPersonalAccounts(db *gorm.DB, cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	if !cfg.Personal.Enabled || cfg.Personal.Instances <= 0 {
		// 关闭持久账号：禁用历史账号（命名与 demoN 区分，避免误伤演示账号）
		return db.Model(&sysmodel.SysUser{}).
			Where("tenant_id IN ? AND username REGEXP ?", managedPersonalTenantIDs(), "^user[0-9]+$").
			Update("status", 0).Error
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.Personal.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	current := make([]string, 0, cfg.Personal.Instances)
	for i := 1; i <= cfg.Personal.Instances; i++ {
		username := cfg.Personal.AccountUsername(i)
		tenantID := cfg.Personal.AccountTenantID(i)
		current = append(current, username)

		// 个人体验角色（每租户一份，幂等：存在则同步权限）
		var role sysmodel.SysRole
		err := db.Where("tenant_id = ? AND name = ?", tenantID, "personal").First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			role = sysmodel.SysRole{TenantID: tenantID, Name: "personal", Perms: personalPerms, Remark: "公开持久体验账号，数据长期保留"}
			if err := db.Create(&role).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&role).Updates(map[string]any{
			"perms": personalPerms, "remark": "公开持久体验账号，数据长期保留",
		}).Error; err != nil {
			return err
		}

		// 个人体验用户（幂等：存在则同步密码/昵称并启用）
		var user sysmodel.SysUser
		err = db.Where("tenant_id = ? AND username = ?", tenantID, username).First(&user).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = sysmodel.SysUser{
				TenantID: tenantID, Username: username, PasswordHash: string(hash),
				Nickname: cfg.Personal.AccountNickname(i), Status: 1,
			}
			if err := db.Create(&user).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if err := db.Model(&user).Updates(map[string]any{
			"password_hash": string(hash), "nickname": cfg.Personal.AccountNickname(i), "status": 1,
		}).Error; err != nil {
			return err
		}

		// 用户-角色关联（幂等）
		var link sysmodel.SysUserRole
		err = db.Where("user_id = ? AND role_id = ?", user.ID, role.ID).First(&link).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := db.Create(&sysmodel.SysUserRole{TenantID: tenantID, UserID: user.ID, RoleID: role.ID}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		// 首次创建该租户时种入初始业务数据（seedDemoData 自带幂等：已有仓库即跳过），
		// 之后访客的改动永久保留。
		if err := seedPersonalData(db, tenantID); err != nil {
			return err
		}
	}

	// 禁用不在当前名单内的历史个人账号（收缩数量后多余的 userN+1..）
	return db.Model(&sysmodel.SysUser{}).
		Where("tenant_id IN ? AND username REGEXP ? AND username NOT IN ?",
			managedPersonalTenantIDs(), "^user[0-9]+$", current).
		Update("status", 0).Error
}

func managedPersonalTenantIDs() []int64 {
	ids := make([]int64, 0, 99)
	var cfg config.PersonalConfig
	for i := 1; i <= 99; i++ {
		ids = append(ids, cfg.AccountTenantID(i))
	}
	return ids
}

// seedPersonalData 给持久租户种入初始业务数据；必须在租户 ctx 内执行，
// 否则计数与写入都会落到 tenant_id=0 的平台租户（而非该个人账号的租户）。
func seedPersonalData(db *gorm.DB, tenantID int64) error {
	ctx := tenant.WithTenant(context.Background(), tenantID)
	return db.WithContext(ctx).Transaction(seedDemoData)
}

// ResetDemoData 硬删除指定租户的演示业务数据并重新写入默认演示数据。
// 用户、角色、迁移记录和操作日志不会删除。
// 租户 ID 经 ctx 传播：删除和种子都由 GORM 租户回调自动限定在该租户内，
// 各演示账号互不影响；tenantID <= 0（默认租户/平台旁路）时保持全局重置的旧行为。
func ResetDemoData(ctx context.Context, db *gorm.DB, tenantID int64) error {
	ctx = tenant.WithTenant(ctx, tenantID)
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
