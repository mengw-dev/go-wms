# 持续审查与优化记录

这是一份阶段记录，不是"全项目已完美"的验收报告。通过现有检查不代表覆盖全部业务组合。

## 1. 当前判断

| 方面 | 判断与依据 |
| --- | --- |
| Go 与可维护性 | 按业务拆包、普通构造函数、具体 Repository 可以保留。主要问题是错误被误转为不存在、后台工作隐式启动和注释夸大保证；已修正多条实际路径。 |
| 架构 | 单体适合库存、单据和任务共用事务。继续保留 handler/service/repository，允许具体模块有局部差异，不增加通用 CRUD 或微服务。 |
| 数据库 | 有版本化迁移、唯一键、行锁和条件更新基础。但聚合计数、裸表租户条件、收货数量语义曾有实际错误。AutoMigrate 与 SQL 迁移仍需持续对照。 |
| Redis | 条码缓存、单号和演示会话用途不同，不能统一宣传"故障都可降级"。条码已按租户隔离；会话操作原子核对 token；单号失败后改用 UUID，任务号直接复用任务主键，移除业务事务内的 Redis 单号调用。 |
| 并发与 WMS | 已有真实 MySQL 防超卖测试；本轮补充盘点锁读差异、租户写入隔离、收货残品、导入执行权等回归。实物盘点冻结、跨实例文件共享等尚不具备。 |
| Docker | 已有多阶段构建、非 root 用户、持久化卷、服务名连接及健康检查；补充忽略各类 .env 文件。 |
| 测试 | 后端全量测试强制使用隔离 MySQL 库执行通过，含 000006 迁移回滚与重新应用。Windows 缺少 C 编译器，未能在本机执行 race；CI 已配置。 |
| 安全 | 修复业务路由绕过演示会话、审计租户丢失与脱敏、Token 租户复核、过期权限缓存继续放行。平台旁路、外部 API Key 和演示重置仍需专项复核。 |

## 2. 已修复的关键问题

优先级表示修复前的影响，不表示这些问题目前仍未修复。

| 级别 | 文件 / 位置 | 原问题、后果与处理 |
| --- | --- | --- |
| P0 | `internal/modules/inbound/service/receiving.go` / Receive | 前端 qty 表示包含不良品的收货总量，后端又加上 defective_qty 计算剩余和任务进度，造成拒收或提前收齐。统一为总收货量，残品只在计算上架量时扣除。 |
| P0 | `internal/modules/stocktake/service/approve.go` / Approve | 差异先普通查询、调整后锁读，可能使用两个账面值。由 Inventory.Adjust 在锁内返回实际应用差异，单据差异与流水一致。 |
| P0 | `internal/modules/inventory/repository/repository.go` / FIFO 与数量写入 | 裸表和原生 Exec 无法依赖 GORM 模型租户回调。FIFO 显式过滤租户及软删，数量更新改为模型 Updates 保留条件防护。 |
| P0 | `internal/modules/inbound/service/import_worker.go` | 上传与补偿启动独立 goroutine，旧执行可覆盖重跑状态。改为受入口管理的单消费者，数据库队列恢复，run_token 校验执行权。 |
| P0 | `internal/app/router.go` / NewRouter | Gin 注册路由后才 Use(DemoSession)，之前注册的业务接口没有会话校验。先挂载中间件再注册业务路由。 |
| P0 | `internal/pkg/middleware/middleware.go` / OperLog | Background 丢失请求租户；原始 query 未脱敏。保留请求 context、脱敏 query、超限立即拒绝。 |
| P0 | `internal/pkg/tenant/gorm.go` / Create 回调 | 正租户上下文中显式指定其他 tenant_id 仍可创建。现在返回 ErrMismatch。 |
| P0 | `internal/pkg/jwt/jwt.go`、`system/service/auth.go` | 补全 issuer、必要过期字段与有效声明校验，数据库复核租户。 |
| P0 | `internal/modules/system/service/permission.go` / cachedPerms | 权限过期后数据库故障仍回退旧权限。改为查询失败拒绝授权。 |
| P0 | `internal/modules/demo/service/session.go` | GET 后 EXPIRE/DEL 会在过期换主窗口操作其他访客的会话。改为 Lua 比较 token 后原子续期/删除/读取 TTL。 |
| P0 | `internal/modules/demo/service/service.go` / demoTenantID | 只检查正租户 ID，拥有演示权限的普通租户也可能进入清空数据的路径。会话领取与重置现在只接受配置的演示租户。 |
| P0 | `internal/modules/system/repository/repository.go` / replaceRoles | 分配角色未核对实际租户。写入前锁定并核对角色，关联使用用户实际租户。 |
| P0 | `internal/pkg/snowflake/snowflake.go` / Next | 系统时钟回拨时序列可能重置并重用 ID。改为单个具体生成器和互斥锁，沿用逻辑时间。 |
| P0 | `internal/pkg/middleware/api_key.go`、`outbound/service/integration.go` | 外部 API Key 入口原本没有租户绑定。现在由服务端配置固定租户，`tenant_id=0` 也使用精确隔离上下文。 |
| P1 | `internal/modules/system/repository/repository.go` / UpdateUser、DeleteRole | 状态单独更新会清空昵称；删除角色先查后删，期间可能被分配。可选昵称使用指针保留字段存在性；检查引用与删除同事务。 |
| P1 | `internal/pkg/orderno/orderno.go` | 单号本地计数跨日重置和跨实例降级有冲突风险。降级改为已有依赖提供的 UUID，Redis 操作设置超时，任务号复用主键。 |
| P1 | `internal/modules/task/service/service.go` / Create、progress | 非法任务被静默跳过，进度相加可能整数溢出。拒绝无效类型、空任务和非正数量，改为剩余量比较。 |
| P1 | `internal/modules/system/service/auth.go` / 登录 | 用户名只在租户内唯一，但登录 First 任意匹配。增加可选租户编号；省略时检测重名并拒绝。 |
| P1 | `internal/modules/system/service/login_limiter.go` | 限流检查与失败计数分离，并发请求可同时通过。锁内预占次数，最多 10000 个记录。 |
| P1 | `web/src/views/login/index.vue` / startDemo | 领取会话失败时已保存 JWT 却没有清理。失败时清空认证信息。 |
| P1 | `internal/modules/outbound/service/order.go` | 并发相同业务单号插入时，唯一键冲突可能被当成订单号冲突反复重试。插入冲突后先按业务单号回查。 |
| P1 | `internal/modules/inventory/service/stock.go` / Allocate | 分配改变可用量却没有 ALLOCATE 流水。补充同事务流水。 |
| P1 | `internal/modules/inventory/repository/repository.go` / SummaryBySKU | total 统计库存批次行而不是汇总后的 SKU。改为 distinct sku_id 计数。 |
| P1 | `cmd/wms`、`system/service/audit.go` | 构造阶段启动消费者、退出路径资源管理不完整。分离 run、HTTP 服务关闭和 dotenv 读取。 |
| P1 | `internal/modules/inbound/service/import_parse.go` | 坏文件、空文件可能显示完成。严格整数校验，明确失败状态，限制解压大小。 |
| P1 | basic/inbound/outbound/stocktake/system 的查询入口 | 数据库错误和 context 取消被当作业务不存在。仅 errors.Is(ErrRecordNotFound) 转业务错误。 |
| P1 | `internal/modules/basic/service/sku.go` | 条码租户内唯一却共用缓存 key。使用租户 key、校验缓存内容，并在更新时使旧 key 失效。 |
| P2 | `internal/pkg/tx/tx.go` | 用错误文本识别 MySQL 类型不可靠。使用 errors.As 读取错误码。 |
| P2 | `docs/architecture.md`、`docs/database.md` | 原文包含不实保证。改为描述实际代码与限制。 |
| P2 | `internal/testutil/mysql.go` | 全量测试中无上限连接池导致 MySQL 1040 错误。限制测试连接池。 |

## 3. 必须保留的边界

- 盘点仍将审核时库存设置成录入的实盘数；没有悄悄改为"按创建快照差异累加"。现场作业未冻结时，两种业务口径不同，需要单独设计。
- 部分导入成功仍标为 COMPLETED，同时返回失败行数和摘要；整体文件错误、全部失败、超时或异常中断标为 FAILED。
- 库存 version 递增不代表使用乐观锁；FIFO 依据首次上架时间，同一库存四元组后续收货会合并。
- 权限缓存仍有有效 TTL，多实例没有统一失效广播。本轮只移除了过期后无限回退旧权限的行为。
- 平台 tenant_id=0 仍有跨租户旁路语义；这项兼容性目前保留，不能把"有回调"当成所有 SQL 都隔离的证明。

## 4. 下一阶段的明确顺序

1. 演示场景执行：会话原子续期和释放已修复，继续复核整个演示场景、重置和普通业务在途请求的并发关系。
2. 平台与身份边界：角色关联、同名账号登录和外部 API Key 租户绑定已修复，继续处理种子账号的跨租户查询和手工平台代操作。
3. 业务完整性：任务并发推进已补回归；继续补出库审核与取消、基础数据删除和库存增加并发、盘点作业时间边界的完整链路测试。
4. 文件和部署：导入文件清理、孤立文件、共享存储、应用数据库最小权限、容器启动及关停验证。
5. 完成其余 Go 文件、前端、配置、脚本和文档逐项核对，再输出最终全项目报告。

## 5. 学习时特别注意

| 不推荐写法或理解 | 当前推荐做法 | 原因 |
| --- | --- | --- |
| 所有查询失败都返回 NotFound | errors.Is 分类，其余返回原错 | 断网、取消和不存在的处理不同 |
| 构造函数里直接 go 一个永远循环 | New 只组装，入口运行 Run(ctx) 并等待 | 可以解释资源归属和退出顺序 |
| Context 只是一项形式参数 | 让请求身份、租户、取消真正传到数据库 | 换成 Background 会丢失隔离与取消 |
| 状态等于 PROCESSING 就代表"我的任务" | 本次执行 token + 条件更新 | 状态无法区分旧 worker 与新 worker |
| 有 version 字段就是乐观锁 | 查看 UPDATE 的 WHERE 是否比较旧版本 | 解释应以执行 SQL 为准 |
| 更长目录和更多接口就是更好的架构 | 保留具体模块和必要接口 | 学习成本和维护成本也是设计约束 |
| 普通 string/int 表达所有更新字段 | 可选字段用指针区分省略和零值 | 切换状态不应顺带清空昵称 |
| 检查没有人使用角色，再单独删除 | 角色分配与删除使用同一行锁协议 | 事务边界要覆盖检查和变更 |
| 给每个共享字段单独加原子操作就安全 | 一个 mutex 保护相关状态 | 多字段组合的不变量需要整体同步 |
| 用 First 查找只在租户内唯一的用户名 | 指定租户，或检测歧义后拒绝 | 查到一条不代表身份唯一 |
| 先检查次数，慢操作失败后才增加 | 在同一临界区检查并预占次数 | 并发请求不能共享同一份剩余额度 |

推荐先阅读：入口 → SKU 查询和错误分类 → 收货数量测试 → 库存分配和流水 → 盘点并发测试 → 导入执行权。目标是能说明每个约束为什么存在，而不是背下文件排版。

## 6. 本地验证与发布注意

已执行：强制隔离 MySQL 的 `go test ./... -count=1`，包括真实并发、事务回滚和全量迁移后 000006 回滚/重新应用；演示会话 Lua 测试连接本地 Redis 兼容服务；`go build ./...`、`go vet ./...`、gofmt 和 golangci-lint。每次 MySQL 测试通过测试辅助函数创建并清理独立 schema。Redis 测试通过 `WMS_TEST_REDIS_ADDR` 显式启用，CI 已配置 Redis 7 服务。

2026-09-30 审查分支补充验证：Docker Compose 全栈构建与启动（应用改用 wms_app 非 root 账户）、Playwright E2E 27 用例、k6 60 并发防超卖（40 成功/20 拒绝，无负库存）、迁移场景 A（全新库）与场景 B（旧版本库 6→8 升级：历史数据保留、新索引与 CHECK 约束生效；000008 遇脏数据 fail-fast 且 dirty，修复数据后 force+up 恢复；000007/000008 down→up 循环通过）。

尚未验证：本地 race（缺少 C 编译器，CI 已覆盖）。

发布前先停止旧版本及导入 worker，运行 000006 迁移，再启动新版本。旧 worker 不认识 run_token，不能混用两种执行方式。历史含残品收货可能已有错误任务进度；需要按原始记录核对，不能直接批量回写线上库存。

Compose 的应用停止宽限期默认改为 45 秒（`WMS_STOP_GRACE_PERIOD` 可覆盖），为 HTTP 退出、导入任务归还和日志排空留出时间。如果增大应用退出超时，应同步调整容器宽限期。

## 7. 历史归档

以下为 2026-09-22 至 2026-09-23 的夜间自动维护摘要，仅保留 commit hash 和一句话摘要，详细过程不再保留。

| 日期 | Commit | 摘要 |
| --- | --- | --- |
| 09-22 | `c2a33f0` | 隐藏 API 序列化中的内部 version 字段 |
| 09-22 | `7f37fa9` | 调整测试敏感字面量消除 gosec 误报 |
| 09-23 | `8007fb3` | system 用户/角色列表改为独立 Response DTO |
| 09-23 | `51f0bad` | 修复仓库和库位列表筛选契约 |
| 09-23 | `42d7ebb` | task 查询新增独立 TaskResp |
| 09-23 | `38c7750` | middleware 按职责拆分为独立文件 |
| 09-23 | `2a13c3a` | inventory 列表和库存流水列表改用 Response DTO |
| 09-23 | `0b5f842` | basic 仓库/库位/SKU 列表改用 Response DTO |
| 09-23 | `3fee4da` | inbound 导入任务查询改用 ImportTaskResp |
| 09-23 | `b140bbd` | inbound 入库单列表和创建改用 OrderResp |
| 09-23 | `5f2ae50` | stocktake 列表/创建/详情改用 Response DTO |
| 09-23 | `1dc8e0a` | outbound 出库单列表和创建改用 OrderResp |
| 09-23 | `19cc340` | system 操作日志查询改用 OperLogResp |
| 09-23 | `747f340` | outbound 详情接口改用 OrderDetailResp |
| 09-23 | `67358ea` | 库存按 SKU 汇总改用 SummaryRow + InventorySummaryResp |
| 09-23 | `fc4fe48` | inbound 详情接口改用 OrderDetailResp |
| 09-23 | `93ba301` | demo scenario 按职责拆分 |
| 09-23 | `36fad10` | demo 并发流程按职责拆分 |
| 09-23 | `d5ae8c9` | demo 并发分配入口命名明确化 |
| 09-23 | `91b2fc4` | config 配置结构拆分为 types.go 和 config.go |
| 09-23 | `c61bde9` | demo 操作记录接口改用显式 Response DTO |
| 09-23 | `c88dce8` | 批量创建库位响应改为 LocationBatchResp |
| 09-23 | `9220250` | bootstrap 种子逻辑拆分为 seed/seed_admin/seed_demo |
| 09-23 | `911579f` | inventory 三数量库存不变量说明注释 |
| 09-23 | `4103101` | 入库任务 ID 可空契约对齐 |
| 09-23 | `4c0d1be` | 修正 race 验证数据库 DSN 的 PowerShell 变量边界 |

以上 commit 均已推送 `main`，GitHub Actions CI 均为 Success。
