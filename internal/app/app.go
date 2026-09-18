package app

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	basicapi "gowms/internal/modules/basic/api"
	basichandler "gowms/internal/modules/basic/handler"
	basicrepo "gowms/internal/modules/basic/repository"
	basicservice "gowms/internal/modules/basic/service"
	demohandler "gowms/internal/modules/demo/handler"
	demoservice "gowms/internal/modules/demo/service"
	aihandler "gowms/internal/modules/ai/handler"
	aiservice "gowms/internal/modules/ai/service"
	inboundhandler "gowms/internal/modules/inbound/handler"
	inboundrepo "gowms/internal/modules/inbound/repository"
	inboundservice "gowms/internal/modules/inbound/service"
	invapi "gowms/internal/modules/inventory/api"
	invhandler "gowms/internal/modules/inventory/handler"
	invrepo "gowms/internal/modules/inventory/repository"
	invsrvc "gowms/internal/modules/inventory/service"
	outhandler "gowms/internal/modules/outbound/handler"
	outrepo "gowms/internal/modules/outbound/repository"
	outservice "gowms/internal/modules/outbound/service"
	stocktakehandler "gowms/internal/modules/stocktake/handler"
	stocktakerepo "gowms/internal/modules/stocktake/repository"
	stocktakeservice "gowms/internal/modules/stocktake/service"
	sysapi "gowms/internal/modules/system/api"
	syshandler "gowms/internal/modules/system/handler"
	sysrepo "gowms/internal/modules/system/repository"
	sysservice "gowms/internal/modules/system/service"
	taskhandler "gowms/internal/modules/task/handler"
	taskrepo "gowms/internal/modules/task/repository"
	taskservice "gowms/internal/modules/task/service"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/lock"
	"gowms/internal/pkg/observability"
	"gowms/internal/pkg/orderno"
	"gowms/internal/pkg/snowflake"
	"gowms/internal/pkg/tx"
)

// App 依赖组装容器：手动构造函数注入，包间仅通过接口通信，预留按包拆分扩展点。
type App struct {
	Config  *config.Config
	DB      *gorm.DB
	Redis   *redis.Client
	Metrics *observability.Metrics

	// 模块对外接口（未来拆分微服务时的边界）
	SystemAPI    sysapi.SystemAPI
	BasicAPI     basicapi.BasicAPI
	InventoryAPI invapi.InventoryAPI

	// Handler 层（供 router 注册路由）
	SysHandler       *syshandler.Handler
	BasicHandler     *basichandler.Handler
	InvHandler       *invhandler.Handler
	TaskHandler      *taskhandler.Handler
	InboundHandler   *inboundhandler.Handler
	OutboundHandler  *outhandler.Handler
	StocktakeHandler *stocktakehandler.Handler
	AIHandler        *aihandler.Handler
	DemoService      *demoservice.Service
	DemoHandler      *demohandler.Handler

	// 供后台任务使用
	InboundService *inboundservice.Service
}

// New 按依赖顺序组装所有模块（无循环依赖：basic→inventory，inbound/outbound→basic+inventory+task）。
func New(cfg *config.Config, db *gorm.DB, rdb *redis.Client, metrics *observability.Metrics) (*App, error) {
	// 节点号来自配置（多实例部署时每实例必须配置不同的 server.node，否则会生成重复雪花 ID）
	snowflake.Init(cfg.Server.Node)
	tm := tx.New(db)
	no := orderno.New(rdb)

	// system：无外部模块依赖
	sysSvc := sysservice.New(sysrepo.New(db), cfg.JWT.Secret, cfg.JWT.ExpireHours)

	// inventory：核心模块，仅依赖事务管理器
	invSvc := invsrvc.New(invrepo.New(), tm)

	// basic：依赖 inventory 暴露的 StockChecker
	basicSvc := basicservice.New(basicrepo.New(), tm, newRedisAdapter(rdb), invSvc)

	// task：统一任务模块
	taskSvc := taskservice.New(taskrepo.New(), no, db)

	// inbound / outbound / stocktake：依赖 basic + inventory + task 接口
	inboundSvc := inboundservice.New(inboundrepo.New(), tm, no, basicSvc, invSvc, taskSvc, cfg.Upload.Dir, lock.New(rdb))
	outSvc := outservice.New(outrepo.New(), tm, no, basicSvc, invSvc, taskSvc)
	stocktakeSvc := stocktakeservice.New(stocktakerepo.New(), tm, no, invSvc)
	aiSvc := aiservice.New(cfg.AI, db, rdb)
	demoSvc := demoservice.New(cfg, db, rdb, inboundSvc, outSvc, stocktakeSvc)

	return &App{
		Config:  cfg,
		DB:      db,
		Redis:   rdb,
		Metrics: metrics,

		SystemAPI:    sysSvc,
		BasicAPI:     basicSvc,
		InventoryAPI: invSvc,

		SysHandler:       syshandler.New(sysSvc),
		BasicHandler:     basichandler.New(basicSvc),
		InvHandler:       invhandler.New(invSvc),
		TaskHandler:      taskhandler.New(taskSvc),
		InboundHandler:   inboundhandler.New(inboundSvc),
		OutboundHandler:  outhandler.New(outSvc),
		StocktakeHandler: stocktakehandler.New(stocktakeSvc),
		AIHandler:        aihandler.New(aiSvc),
		DemoService:      demoSvc,
		DemoHandler:      demohandler.New(demoSvc),

		InboundService: inboundSvc,
	}, nil
}

// redisAdapter 将 go-redis 适配为 basic 模块定义的 redisClient 接口（依赖倒置）。
type redisAdapter struct{ rdb *redis.Client }

func newRedisAdapter(rdb *redis.Client) *redisAdapter { return &redisAdapter{rdb: rdb} }

func (a *redisAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.rdb.Get(ctx, key).Result()
}

func (a *redisAdapter) Set(ctx context.Context, key string, val any, ttl time.Duration) error {
	return a.rdb.Set(ctx, key, val, ttl).Err()
}

func (a *redisAdapter) Del(ctx context.Context, keys ...string) error {
	return a.rdb.Del(ctx, keys...).Err()
}
