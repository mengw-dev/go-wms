# WMS 代码索引（code-index）

> 本文件只做**模块级导航**：模块在哪个目录、大致负责什么、对应哪些测试。
> 文件内部的 `struct` / `interface` / 函数实现**一律以代码为准**；若本文件与代码不一致，以代码为准。

## 阅读方式

- 本文件不描述单文件实现细节，回答的是"改某个模块该看哪里"，不是"某个函数做了什么"。
- 改某个模块前：先看本文件的该模块小节与其列出的测试文件，再读真实实现，最后动手。
- 不允许根据文件名猜逻辑；不确定时先查调用链（grep 调用方/被调用方）与测试，再下结论。
- 命令、环境变量、部署步骤不在本文件重复，直接看文末链接指向的文档。
- 路径约定：本文件路径相对仓库根目录；`web/src/**` 小节内的路径相对 `web/src/`。
- 相关文档：[文档目录](README.md)、[Go 约定](go-style.md)、[架构说明](architecture.md)、[数据库设计](database.md)。职责与依赖规则统一维护在 Go 约定中，本索引不重复实现细节。

## 技术栈与顶层目录

- 后端：Go（Gin + GORM + MySQL + Redis + Viper）；入口 `cmd/wms/main.go`，迁移工具 `cmd/migrate/main.go`。
- 前端：Vue 3 `<script setup>` + TypeScript + Pinia + Vue Router 4 + Element Plus + Vite（axios）。
- 顶层目录：
  - `cmd/` 可执行入口（wms 服务、migrate 迁移）
  - `internal/` 后端代码：`app`（组装/路由）、`bootstrap`（DB/迁移/种子）、`modules/`（业务模块）、`pkg/`（通用能力）、`testutil/`（测试基础设施）
  - `migrations/` 内嵌 SQL 迁移；`configs/` 配置；`deploy/` 部署
  - `scripts/` 起栈/验证/E2E/压测；`web/` 前端工程；`docs/` 文档；`loadtest/`、`samples/` 辅助目录

## 一、后端核心业务模块（禁止随意重构）

> 覆盖 `internal/modules/{inbound,outbound,task,inventory,stocktake}`、`internal/pkg/{tenant,lock,tx}`、`migrations/**`。
> 这些涉及**库存一致性、FIFO 分配、并发扣减、多租户隔离**，改动前必须确认调用链与不变量。

| 模块 | 目录 | 职责 | 对应测试 |
| --- | --- | --- | --- |
| 入库 inbound | `internal/modules/inbound` | 入库单生命周期、收货、上架、Excel 异步导入、资源清理及队列观测 | `service/{order,receiving,query,import,import_security,import_cleanup,import_recovery,import_metrics,response,putaway_location_race,import_completed_cleanup,fingerprint_compatibility}_test.go`、`dto/dto_test.go`、`handler/handler_test.go`、`model/model_test.go` |
| 出库 outbound | `internal/modules/outbound` | 出库单生命周期、审核 + FIFO 分配、拣货、发货、集成下单 | `service/{order,response}_test.go`、`model/model_test.go`、`handler/handler_test.go` |
| 任务 task | `internal/modules/task` | 收货/上架/拣货任务的状态与进度 | `service/{service,response,dashboard_filter}_test.go` |
| 库存 inventory | `internal/modules/inventory` | 库存汇总/明细、并发分配与扣减、调整、流水 | `service/{service,concurrency,response,availability}_test.go`、`repository/repository_test.go` |
| 盘点 stocktake | `internal/modules/stocktake` | 盘点单、账面快照、实盘录入、差异、审核调整 | `service/{service,response,fingerprint_compatibility}_test.go`、`handler/{actual,list}_test.go`（实盘参数与列表查询契约）、`model/model_test.go` |

核心基础设施（同样禁止随意重构）：

| 能力 | 目录 | 职责 | 对应测试 |
| --- | --- | --- | --- |
| 多租户隔离 | `internal/pkg/tenant` | 租户上下文与 GORM 过滤；区分平台旁路和精确租户（包括租户 0） | `gorm_test.go` |
| 分布式锁 | `internal/pkg/lock` | 基于 Redis 的锁（`SET NX EX` + Lua 校验持有者释放） | — |
| 事务与重试 | `internal/pkg/tx` | 事务管理器、死锁重试与退避及耗时/重试观测 | `tx_test.go`、`mysql_test.go`、`metrics_test.go` |
| 请求幂等 | `internal/pkg/idempotency` | 幂等记录写入/查询及有批数/时间预算的过期清理（表 `wms_idempotency`） | `fingerprint_test.go`（版本/字节边界/旧格式安全兼容）、`cleanup_test.go`（分批/隔离/并发/超时/恢复/调度指标）、`internal/app/pick_race_test.go`（领取/拣货/清理集成测试） |
| 数据库迁移 | `migrations/**` | 版本化 SQL 迁移，`embed.go` 内嵌 `versions/*.sql` | `migrations_test.go`、`quantity_test.go`、`checks_test.go` |

跨模块关键落点（仅记位置，细节以代码为准）：

- 库存一致性与并发：`internal/modules/inventory/repository`（悲观锁 + 条件更新 CAS）。
- 出库 FIFO：`internal/modules/outbound/service` → `internal/modules/inventory/api`，具体 Service 由 `internal/app` 装配。
- 拣货：`internal/modules/outbound/service/pick.go`。
- Excel 异步导入：`internal/modules/inbound/service`（导入 worker + `run_token` 防重复）。
- 收货/上架联动：`internal/modules/inbound/service/{receiving,putaway}.go`。

## 二、后端平台与基础设施

| 模块 | 目录 | 职责 | 对应测试 |
| --- | --- | --- | --- |
| 系统 system | `internal/modules/system` | 鉴权（登录/JWT）、权限、用户、角色、操作日志 | `service/{auth,permission,user,role,login_limiter,builtin,audit,response,p2_auth}_test.go`、`dto/dto_test.go`、`model/model_test.go` |
| 基础数据 basic | `internal/modules/basic` | 仓库、库位、SKU | `service/{sku,delete,rows_affected,conflict,response,location_limits}_test.go`、`dto/dto_test.go`、`handler/handler_test.go`、`repository/repository_test.go`、`model/model_json_test.go` |
| 仪表盘 dashboard | `internal/modules/dashboard` | 按业务权限/仓库汇总当前状态、创建趋势、最早待办与库存/库位概况，只读事务 | `service/overview_test.go`；前端 E2E `dashboard-layout.spec.ts` |
| AI ai | `internal/modules/ai` | 库存问答、Redis 原子额度、整次预算及主备取消（智谱 BigModel） | `service/{service,low_stock,reliability}_test.go`；前端 E2E `ai-reliability.spec.ts` |

应用装配与启动：

| 路径 | 职责 | 对应测试 |
| --- | --- | --- |
| `internal/app` | 依赖组装与路由注册 | `router_test.go`、`integration_test.go`、`list_contract_test.go`（业务列表HTTP/SQL契约）、`race_cleanup_test.go`（门闩/取消/worker回收）、`pick_race_test.go`、`order_references_test.go`、`order_quantity_test.go`、`task_pagination_test.go`、`external_identity_test.go`、`schema_contract_test.go`、`fingerprint_compatibility_test.go`（领取/拣货旧记录回放及字段移位拒绝） |
| `internal/bootstrap` | DB/Redis 初始化、迁移、种子（管理员/演示/体验账号）、演示数据重置 | `seed_test.go`、`personal_test.go`、`reset_test.go`、`quantity_test.go`、`checks_test.go` |
| `cmd/wms` | 服务入口、`.env` 加载、优雅关停、后台 worker 启动 | `dotenv_test.go`、`server_test.go` |
| `cmd/migrate` | 迁移工具与管理员/应用账户初始化 | `preflight_test.go` |
| `internal/testutil` | 测试用 MySQL/Redis 基础设施（临时 schema、租户回调、正式迁移入口、Redis 命名空间） | `mysql_test.go`、`redis_test.go`；应用结构契约见 `internal/app/schema_contract_test.go` |

通用能力 `internal/pkg/**`：

| 包 | 职责 | 对应测试 |
| --- | --- | --- |
| `config` | 配置加载（viper，`WMS_` 前缀环境变量覆盖） | `config_test.go` |
| `jwt` | JWT 生成与解析 | `jwt_test.go` |
| `middleware` | 鉴权、API Key、Demo 会话、请求上下文、日志、恢复、CORS 等中间件 | `api_key_test.go`、`audit_test.go`、`demo_test.go` |
| `response` | 统一响应体 | `response_test.go` |
| `errcode` | 错误码与冲突判断 | `errcode_test.go` |
| `httpx` | HTTP 参数解析辅助 | `params_test.go` |
| `quota` | 租户配额守卫 | `quota_test.go` |
| `snowflake` | 雪花 ID | `snowflake_test.go` |
| `orderno` | 单号生成 | `orderno_test.go` |
| `observability` | Prometheus HTTP/事务/导入指标与 MySQL 实例行锁观测 | `metrics_test.go`、`row_locks_test.go` |
| `typex` | ID 集合类型（跨 JSON 数字与字符串场景） | `idlist_test.go` |
| `modelbase` | GORM 模型基类 | — |
| `dbutil` | 数据库查询辅助 | — |
| `log` | 结构化日志入口（带请求上下文字段） | — |
| `version` | 版本信息 | — |

## 三、Demo / 外挂层

> 依赖方向约定：业务页面只能通过 `web/src/events/businessEvents.ts` 与 Guide/Demo 交互，
> 方向为 **业务页面 → businessEvents ← Guide/Demo**，**禁止反向依赖**。

| 位置 | 职责 | 对应测试 |
| --- | --- | --- |
| `internal/modules/demo` | 演示账号会话与租约、场景编排、并发实验、活动证据、性能数据 | `handler/{handler,json}_test.go`、`service/{session,tenant_lock,scenario,activity,identity,experiment_allocation}_test.go`；种子 `internal/bootstrap/{seed_demo,reset}_test.go` |
| `web/src/guide/**` | 引导定义、路由匹配、持久化、与业务事件的桥接 | `routeMatcher.spec.ts`、`businessBridge.spec.ts` |
| `web/src/components/demo/**` | 演示/引导组件（`ManualGuide`、`DemoRunViewer`、`DemoTour`、`StagedDemoRunner`） | — |
| `web/src/views/demo/**` | 演示页面（`index`、`Activity`、`Performance`） | E2E `web/tests/e2e/demo-console.spec.ts` |
| `web/src/composables/demo/**` | 演示/引导组合式函数（会话、运行器、引导目标等） | `composables/demo/{useDemoRunner,useStagedDemoRunner}.spec.ts`、E2E `demo-scroll.spec.ts`、`api/demoExecution.spec.ts`、E2E `demo-console.spec.ts` |

## 四、前端（web/src）

> 业务页面与 Demo/Guide 的关系见上一节；`events/businessEvents.ts` 是两者唯一契约。

| 目录/文件 | 职责 | 对应测试 |
| --- | --- | --- |
| `api/` | 各业务模块 HTTP 客户端与类型（`api/*.ts`、`api/types/*.ts`） | — |
| `stores/` | Pinia 状态（`auth` 登录态、`guide` 运行态、`theme`） | `stores/auth.spec.ts`、`stores/guide.spec.ts` |
| `router/` | 路由与登录/权限守卫（`router/index.ts`） | — |
| `layouts/` | 布局与导航（`Layout.vue`、`WmsNavigation.vue`）；业务路由独立启用 `business-layout.css`，仪表盘/演示保持原样 | E2E `ui-layout.spec.ts`、`business-layout.spec.ts`、`demo-home-layout.spec.ts` |
| `composables/` | 组合式函数及幂等公共支持：`operationLifecycle`、`autoRefresh`、`demo/useDemoDock`、`inbound/*`、`stocktake/useStocktakeApprove`、`outbound/usePdaPick`、`demo/*` | `operationLifecycle.spec.ts`、`autoRefresh.spec.ts`、`inbound/useInboundOrders.spec.ts`（手动/后台查询交错）、`inbound/{useReceive,usePutaway,useInboundImport}.spec.ts`、`stocktake/useStocktakeApprove.spec.ts`、`outbound/usePdaPick.spec.ts` |
| `events/` | 业务页面与 Guide/Demo 的唯一契约（`businessEvents.ts`） | `businessEvents.spec.ts` |
| `utils/` | 前端工具（事件、演示证据/操作、分步检查点、options、手机表格字段标签等） | `utils/{demoOperations,demoEvidence,index}.spec.ts`、`composables/demo/useStagedDemoRunner.spec.ts`；E2E `business-layout.spec.ts` |
| `views/` | 业务页面（`login`、`dashboard`、`basic`、`system`、`task`、`ai`、`inventory`、`inbound`、`outbound`、`stocktake`、`demo`） | E2E 见 `web/tests/e2e/**` |
| `components/` | 组件（`DemoConsole`、`demo/*`、`PickDialog`、`PutawayDialog`、`ReceiveDialog`、`common/{PageHeader,BusinessListPage,BusinessFilters,BusinessPagination}`）；筛选折叠/手机分页只管理呈现，业务事件继续转发 | E2E `business-layout.spec.ts` |
| `directives/` | `permission` 指令（按权限控制显示） | — |
| `main.ts` / `App.vue` / `style.css` / `constants.ts` | 应用装配、根组件、全局样式与常量 | — |

## 五、测试、脚本与部署

- 后端单测：`**/*_test.go`（`internal/**`），依赖 MySQL/Redis 时由 `internal/testutil` 创建独立测试库。运行方式见 [development.md](development.md)。
- 前端单测：`web/src/**/*.spec.ts`、工具规则回归 `web/eslint-rules/**/*.spec.mjs`（Vitest，配置 `web/vitest.config.ts`）。
- 端到端：`web/tests/e2e/**`（Playwright，`support/api.ts` 为共享工具层，配置 `web/playwright.config.ts`）；入口脚本 `scripts/e2e.ps1`。
- 脚本：`scripts/windows/*.ps1`（起栈、停止、重置、验证、打包、k6、监控等，多数附 `.cmd` 双击入口）、`scripts/wms-common.ps1`、`scripts/e2e.ps1`。
- 压测：`scripts/k6/*.js`（业务流程与压力场景、分配证明后的范围清理）与 `loadtest/01-hello.js`；清理回归 `scripts/k6/cleanup-allocation{,.k6}.test.mjs`（Node隔离夹具与真实k6回环HTTP）；说明见 [load-testing.md](load-testing.md)。
- 部署：`deploy/`（`docker-compose.yaml`、`docker-compose.dev.yaml`、`Caddyfile`、Prometheus、Grafana）、根 `Dockerfile`、`web/Dockerfile`、`Makefile`、`start.cmd`；说明见 [deployment.md](deployment.md)。
- CI：`.github/workflows/ci.yml`、`.github/workflows/deploy.yml`；`scripts/ci/{changes,policy}.mjs` 负责变更分流、最终结果和部署门禁，回归 `scripts/ci/policy.test.mjs`。验证门禁：`scripts/windows/test-go.ps1`（测试依赖与跳过判定）、`scripts/check-vulns/`（扫描执行与报告策略），回归见 `scripts/tests/verification-gates.tests.ps1`、`scripts/check-vulns/main_test.go`。
- Go 静态检查：`.golangci.yml`（错误返回、资源、依赖代码质量）；资源关闭策略与例外见 [Go 约定](go-style.md)。
- 依赖护栏：`scripts/check-dependencies/`（全量 Go 直接导入检查，接入 CI/verify/Go 测试）、`web/eslint-rules/`（业务与 Demo/Guide 边界，接入 ESLint/Vitest）；合法例外与范围见 [Go 约定](go-style.md)。
- 发布：`scripts/deploy.sh`（指定提交、镜像 digest 与就绪/版本核对）、`scripts/lib/deploy-services.sh`（先迁移再切换应用）、`deploy/docker-compose.release.yaml`；回归见 `scripts/tests/deploy.tests.sh`、`scripts/tests/deploy-containers.sh`。
- 文档：`docs/*.md` 与 `docs/index.html`（架构展示页，源文件 `web/public/overview.html`，在 `web/` 执行 `npm run sync:overview` 同步）。

## 六、常用入口与文档

- 本地开发、测试依赖与提交前校验：见 [development.md](development.md)。
- 部署、迁移与升级：见 [deployment.md](deployment.md)。
- 环境变量与 Compose 账户：见 [configuration.md](configuration.md)。
- 演示体验流程与账号：见 [demo.md](demo.md)。
- 监控（Prometheus / Grafana）：见 [monitoring.md](monitoring.md)。
- 生产就绪评估：见 [production-readiness.md](production-readiness.md)。
- 前端业务页面只通过 `web/src/events/businessEvents.ts` 与 Guide/Demo 交互；架构展示页源文件变更后需执行 `npm run sync:overview`。

## 七、维护说明

- 本文件只保留模块级信息（模块 → 目录 → 职责 → 测试）；新增实现细节请写到架构 / 数据库 / 开发等专题文档。
- 修改代码后请同步更新对应模块小节；不确定的内容不要写成结论，按"以代码为准"处理。
- 面向 AI 的项目导航另见根目录 `AGENTS.md`（本地文件，已忽略，不提交远端）。
