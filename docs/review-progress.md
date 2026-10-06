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
- 请求级幂等尚未实现：出库建单（`biz_order_no`）与导入建单（`import_task_id + import_row`）已有业务键幂等，但收货、拣货是增量写且没有客户端请求标识，单据处于 RECEIVING/PICKING 时重复提交同一请求会再次累加。状态机与 version 只防非法流转和丢失更新，不能替代请求幂等；影响范围、触发路径与规划方案见 `docs/requirements.md` 第 6 章。
- 库存分配的候选查询（`ListFIFOCandidates`）是普通读，用的是事务快照，看不见本次事务开始后才入库的新行。极端情况下（审核事务开始后有人上架补货）可能报一次"可用不足"，下一次请求就能看到新库存。旧实现用 `FOR UPDATE` 读最新已提交数据，没有这个差异——这是为缩小锁范围付出的代价。

## 4. 下一阶段的明确顺序

1. 演示场景执行：会话原子续期和释放已修复，继续复核整个演示场景、重置和普通业务在途请求的并发关系。
2. 平台与身份边界：角色关联、同名账号登录和外部 API Key 租户绑定已修复，继续处理种子账号的跨租户查询和手工平台代操作。
3. 业务完整性：任务并发推进已补回归；继续补出库审核与取消、基础数据删除和库存增加并发、盘点作业时间边界的完整链路测试。
4. 文件和部署：导入文件清理、孤立文件、共享存储、应用数据库最小权限、容器启动及关停验证。
5. 完成其余 Go 文件、前端、配置、脚本和文档逐项核对，再输出最终全项目报告。
6. 请求级幂等（待解决）：为收货、拣货、盘点审核增加 `Idempotency-Key` 与幂等表，插入与业务写操作放在同一事务，并同步前端与测试。

以上是方向性的顺序，可直接照着执行的清单见第 8 节。

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

## 8. 迭代清单（按执行顺序）

护栏类的改动不碰业务行为，风险最低，排在前面；真正改业务的放第二段；结构整理最后做，不影响当前正确性。

### 8.1 护栏

- 删除 `internal/pkg/concurrent`。全仓没有任何地方 import 它，`SafeGo` / `SafeGoNoCtx` 也没有调用方。`unused` 不检查导出符号，所以一直没被 lint 发现。
- 删除 `internal/app/app.go` 里的 `BasicAPI`、`InventoryAPI` 字段，只有赋值、没有读取。
- 给跨模块接口补编译期断言，放在实现包内部而不是集中到一个文件，例如新建 `basic/service/contracts.go` 写 `var _ basicapi.BasicAPI = (*Service)(nil)`。
- `internal/bootstrap/database.go` 用 `strings.Contains(err, "duplicate")`、`"1061"` 判断约束是否已存在，改成 `errors.As` 取 `*mysql.MySQLError` 判 1061。
- 修 `docs/architecture.md` 的盘点描述。它写着"未填写明细跳过"，实际 `stocktake/service/approve.go` 要求每条明细都填了实盘数才能审核。顺便把口径写清楚：创建时不冻结库存，审核时按锁读到的当前库存调整。
- 在 `docs/code-index.md` 增加一节文件职责与依赖方向，把 8.5 的约定落成文字。
- 依赖方向用 golangci-lint 自带的 depguard 配，不要自己去写 AST 脚本。CI 已经在跑 golangci-lint v2.13.2，加配置即可。要守住的方向：handler → service → repository → model；api 不依赖 service、repository、handler；pkg 不依赖 internal/modules；业务模块不依赖 demo，demo 可以调业务 service。
- 事务归属写进规则并让 CI 拦住：事务只在 service 层开，repository 只能用传进来的 `*gorm.DB`。注意 `system/repository/role.go` 和 `user.go` 现在就自己开了 `Transaction`，这条规则定下来要连这两个文件一起改。
- 原生 SQL 的租户要求限定为新增代码：新写的 Raw / Table / Exec / Scan 必须明确租户条件，或显式标注为系统级查询，并补租户测试。历史那批已经在 P0 修过，不要重翻。
- 错误分类写成规则：只把 `gorm.ErrRecordNotFound` 转成业务"不存在"，其它数据库错误原样返回。现状基本符合，只有 database.go 那一处要改。
- lint 分阶段启用：gocyclo、nestif 可以直接开，funlen 先放宽阈值，dupl 先只提示不阻断，lll 不加。用 `issues.new-from-rev` 让复杂度检查只作用于新增和改动的代码，避免历史文件一次性冒出几百条。
- `internal/modules/inbound/service/import_worker.go` 当前有一处只删空行的改动，单独提交掉，不要和后面的重构混在一个 commit 里。

### 8.2 业务正确性

- 请求级幂等。收货、上架、拣货、盘点审核都是有副作用的命令，统一按 `Idempotency-Key` + `tenant_id` + `scope` + `request_hash` 处理，唯一键取 `(tenant_id, scope, idempotency_key)`。key 相同且指纹相同算重复成功；key 相同但指纹不同返回 409；业务失败时幂等记录随事务一起回滚；插入必须放在 `TxRetry` 的回调里。前端重试要复用同一个 key，不能重新生成。这几个接口目前返回空数据，暂时不需要缓存响应体，但要把这个限制写进文档。`Idempotency-Key` 还要加进 CORS 的 Allow-Headers，`middleware/cors.go` 现在只放了 Authorization、Content-Type、X-Request-ID、X-Demo-Session。
- 外部出库单的同号不同内容问题。`outbound/service/integration.go` 只看 `biz_order_no` 就返回原单，OMS-001 先推 SKU A 两件、再推 SKU B 一百件，第二次会被当成重复成功。按规范化后的 `tenant_id + biz_order_no + warehouse_id + details + remark` 算指纹：指纹一致返回原单，不一致返回 409，不同租户允许同号。时间戳、随机数不要进指纹。
- Web 健康检查。nginx 的 `/healthz` 是静态返回 200，后端挂了它照样通过，而部署脚本探的正好是这个地址。要么让它反代到后端健康接口，要么另开一个 `/api-healthz` 给部署探。
- 补并发和回滚测试：`pick`、`putaway` 目前连测试文件都没有；再补盘点期间并发出入库，以及死锁重试和事务回滚。后两项属于正确性测试，和"指标有没有上报"分开。

### 8.3 测试与可观测性

- 导入相关测试补齐：文件缺失、失败文件清理、Worker 重启。
- 补业务指标：死锁重试次数、事务冲突次数、行锁等待、导入任务耗时、FAILED 数量和 PENDING 积压。注意现有的 `wms_db_wait_*` 是连接池等待，不是 MySQL 行锁等待。这项优先级低于前面几项。
- 索引和 N+1 查询审计，需要时留 EXPLAIN 记录。
- API、前端类型和接口文档之间的字段同步检查。

### 8.4 结构整理（不做主线）

- demo 按外挂层处理，不补 repository / dto / api。写清楚：demo 可以编排业务 service、可以有自己的响应结构，业务模块不能依赖 demo，demo 也不作为领域分层的范例。
- `OrderTaskResp` 先不合并。inbound 和 outbound 现在字段一样，但归属不同；确认它确实是稳定的公共任务协议之后，再挪到 task/dto。不要建 common/dto、common/response 这类包。
- `batchOper` 在两处重复，先留着，等出现第三个真实调用方再抽。
- 前端 `utils/events.ts` 里的 demo 事件，以及 `utils/demoEvidence.ts`、`utils/demoOperations.ts`、`utils/demoScenario.ts`，移入 `src/demo/`；`components/DemoConsole.vue` 移入 `components/demo/`。
- 命名统一只从新增和改动的文件开始，不做全量重命名。

### 8.5 文件职责与依赖约定

| 目录 | 放什么 |
| --- | --- |
| `modules/*/model` | 数据库模型、状态枚举、持久化字段 |
| `modules/*/dto` | HTTP 请求与响应结构，不放业务逻辑 |
| `modules/*/api` | 跨模块的稳定合约，不放数据库实现 |
| `modules/*/repository` | 查询、写入、锁读、持久化 |
| `modules/*/service` | 用例编排、校验、事务边界 |
| `modules/*/handler` | HTTP 绑定、鉴权、调用 service、返回响应 |
| `internal/pkg` | 有多个真实使用方的技术基础能力 |
| `modules/demo` | 明确标记的外挂编排层 |
| `cmd`、`internal/bootstrap` | 启动与依赖注入，不放业务逻辑 |

另外几条：

- 接口只在有真实消费者时才定义。只有一个调用方就在调用方那边写个小接口；要跨模块稳定复用才放 api 包。不要给每个 service 都机械地配一个接口。
- 一个导出的核心类型一个文件；强相关的私有结构体和方法可以放一起。按用例拆 `receiving.go`、`putaway.go`、`query.go` 是合理的，文件到三四百行或同时管几件事再拆。
- 抽公共包要同时满足四个条件：有两三个真实调用方、概念稳定、错误和事务语义一致、抽完不会增加模块耦合。不要新增 common、utils、helper、base、shared 这类没有明确职责的包。

### 8.6 暂不处理（登记）

这些是当前的能力边界，不是缺陷，但文档和简历里不要写成"已支持"：

单实例下的本地导入文件目录、多实例的权限缓存与登录限流、Worker 共享存储、Snowflake 跨重启、软配额、best effort 操作日志、API Key 轮换、JWT 存 localStorage、密码强度、AI 库存数据外发与 Redis 故障时限流放行、备份与 PITR、滚动发布与回滚、脚本治理。

Snowflake 那条尤其注意，只能说"单实例加唯一节点号下可用"，不能写"跨重启绝对唯一"。

### 8.7 新增功能的固定动作

读 code-index 和相关测试 → 想清楚类型放哪、依赖往哪走 → 明确事务和租户边界 → 判断要不要幂等 → 补业务测试 → 同步 API 和架构文档 → 跑 gofmt、go vet、go test 和 lint。

## 9. 出库审核与拣货：核对结论与待完善项

这一节只针对出库审核和 PDA 拣货两条链路，结论都对照过代码。

现状判断：仓库侧的基础是可靠的。审核里的 FIFO 分配、PICK 任务生成、出库单状态推进，以及拣货时的任务进度、分配行累加、拣满扣库存，都在同一个事务里，不会超卖也不会超拣，回滚正确。问题集中在三处：锁的范围偏大、这几张表缺少与库存同等强度的数量约束、PDA 作业能力还停留在后台管理系统的形态。

### 9.1 已核实的问题

- `FindFIFOForUpdate`（`inventory/repository/repository.go`）锁住该仓库该 SKU 下所有可用库存行，没有按本次需求收敛；查询里还 JOIN `wms_location` 取 `location_code`，把库位表牵连进锁定读。一个只要 2 件的订单也可能锁住几百行。**已修复**，见 9.2 第 8、9 条。
- `Increase`（`inventory/service/stock.go`）每次入库先锁 warehouse → location → sku。同一仓库下不同库位、不同 SKU 的上架会竞争同一个 warehouse 行。这个锁是为了堵"基础资料删除与库存创建"的竞态，是刻意的，不要当缺陷删。
- 审核按 SKU 排序加锁（`outbound/service/order.go`），盘点按 InventoryID 排序。两条链路对同一批库存的加锁顺序不同，目前靠 `TxRetry` 重试 1213 兜底。
- `TxRetry`（`pkg/tx/tx.go`）只重试 1213 和业务冲突码，1205 锁等待超时不重试；重试耗尽后返回的是原始 MySQL 错误，到 HTTP 层变成 500。现场看到的是"系统内部错误"，不是可识别的冲突。
- 拣货先锁整张出库单（`outbound/service/pick.go`）。同一张单上的多个任务因此串行，一张 40 个任务的大单，40 个 PDA 也要排队；不同订单之间可以并行。**已修复**：改为锁任务行，主单状态由条件原子更新收口；同单多任务可并行，k6 对比见 9.5。
- 拣货时同一个任务被锁两次：先 `taskAPI.GetForUpdate`，随后 `AddProgress` 内部再锁一次。多一次往返，不产生额外等待。**已修复**：`AddProgress` 复用调用方已持有的任务行锁。
- `IncrAllocationPicked`（`outbound/repository/repository.go`）条件只有 id + version + status，没有 `picked_qty + delta <= allocated_qty`。正常路径靠任务剩余量挡住，缺数据库层兜底。
- 拣货不重新校验聚合关系：没有校验 `allocation.order_id == order.id`、`allocation.sku_id == task.sku_id`、`allocation.allocated_qty == task.target_qty`，数据库也没有外键。**已修复**：锁内比较这三处整数关系，不一致返回 40016（明细/库存等边仍由各自锁读与条件更新约束）。
- 单据详情固定取 `DetailTaskPageSize = 200`（`task/api/api.go`），超过 200 个任务的单据看不全。
- `PickDialog.vue` 的作业库位是只读展示，只有批次能扫；后端 `checkPickScan` 对库位和批次都是"传了才校验"，客户端可以不传扫描信息直接提交。注意批次在任务有批次时前端是强制核对的，这里缺的只是库位。**后端已补**：新增 PDA 入口强制库位/批次（缺库位 50010、缺批次 50011）；前端 PDA 页面待做。
- 拣货成功返回 `data: null`，失败只给错误码。第二个 PDA 拿到"数量超过任务剩余数量"时看不到最新进度，只能退出重进。**已修复**：成功与业务拒绝均返回任务快照（`task_status`/`done_qty`/`remaining_qty`/`order_status`），幂等重放回放首次快照。

### 9.2 待完善项

先补正确性和兜底：

1. 拣货请求幂等，与 8.2 是同一件事，不要开两条线。**已完成**：`wms_idempotency` 表 + `Idempotency-Key`（CORS 已放行请求头），重放回放首次成功的结果快照；同 key 不同内容 409。
2. PDA 入口强制库位扫描，服务端校验库位、SKU、批次一致；后台管理入口保留较宽松的人工语义。**已完成**：`POST /pda/tasks/:id/pick` 强制库位（任务有批次时强制批次），后台入口保持"传了才校验"。
3. 拣货时补 allocation / order / task 三者的 id、sku、数量对应关系校验。**已完成**：锁内比较任务 ↔ 分配行的 `order_id`/`sku_id`/`allocated_qty`，不一致返回 40016。
4. 给 task、allocation、shipment_order 补数量 CHECK：`done_qty <= target_qty`、`picked_qty <= allocated_qty`。库存表在 000008 已有同类约束，这三张表还没有。
5. 补拣货的并发、重复请求、取消竞态、事务回滚测试。**已完成**：`internal/app/pick_race_test.go` 覆盖取消先到/拣货先到/同时到三条竞态与 PDA 领取/幂等用例。
6. 死锁重试耗尽和 1205 转成明确的 409/503，不要落到 500；退避加少量随机抖动。

再优化锁和指标：

7. 给 Approve、Pick、Allocate 补耗时、重试次数、锁等待指标，先有数据再决定动不动锁。
8. 把库位编码查询从锁定读里拆出去。**已完成**：`ListFIFOCandidates` 负责非锁定取候选、`LockInventoryByIDs` 只锁前缀，库位编码改由 `ListLocationCodes` 普通读补齐。
9. FIFO 锁读改成逐批取候选、锁够本次数量即可，锁定后重算可用量，不足整体回滚。**已完成**：新增 `TestAllocateOnlyLocksNeededRows`（未使用的行不被锁）与 `TestAllocateAcrossManyBatches`（跨批次 FIFO）覆盖。
10. 审核增加单据明细数和单次分配规模上限。
11. 评估 `Increase` 是否每次都要锁 warehouse 和 SKU：已存在库存行时只锁库存行，首次创建再做基础资料竞争保护。改之前必须先有第 7 项的指标和第 5 项的测试，否则容易把已经解决的删除竞态重新打开。
12. 是否取消拣货的整单锁，取决于"同一大单多 PDA 并发"是不是目标。这不是删一行锁，取消与拣货同时发生的竞态需要重新设计，靠任务和分配行的条件原子更新加订单状态 CAS 来兜。

PDA 能力，属于新功能不是缺陷：

13. 独立的 PDA 任务列表和"下一个任务"接口，按库区、路线排序。
14. 拣货接口返回最新状态：`task_status`、`done_qty`、`remaining_qty`、`order_status`。**已完成**：成功/业务拒绝/幂等重放三条路径都返回任务快照。
15. 任务领取/租约、设备绑定、`claimed_at`、`lease_expire_at`。**已完成（部分）**：`claimed_by`/`claim_token`/`lease_expire_at` + `POST /pda/tasks/:id/claim`（无租约/过期/本人可领，成功拣货自动续租）；设备绑定与 `claimed_at` 未做。
16. 短拣、缺货、破损等异常原因与重新分配流程。

### 9.3 现在不要做

- 不要为了消除死锁去拆审核事务。审核必须是一个原子事务，拆开会出现"库存已分配但没有任务"的半成功状态。
- 不要盲目删行锁或改整单锁。先补第 7 项的指标和第 5 项的测试，用数据决定。
- 不要现在引入波次分配或异步分配。那属于大波次的设计，普通审核先做规模限制就够。
- 和企业 WMS 的差距（任务租约、波次与区域路线、短拣异常、扫描事件审计、锁等待与死锁监控、大订单异步分配、更细的批属性和箱码托盘码）属于现场复杂度，不是当前实现的错误，登记即可。

### 9.4 当前状态与登记项的设计结论（2026-10-05）

- 状态：①（拣货整单锁）与 ⑤（双锁合并）已实现并通过数据库验证（竞态测试 + k6 对比，见 9.5）；②③④（幂等、关系校验、PDA 强制扫描、任务快照、领取租约）也已实现并验证。**仍未提交**。
- **幂等 × 任务快照**：拣货返回任务快照后，幂等重放返回**首次执行时缓存的响应**（不读库），保证"同一请求同一响应"；正常成功返回提交时刻快照；业务拒绝返回当前最新快照（供 PDA 刷新）。幂等命中需短路在领取凭证校验之前，避免重试因凭证过期被误拒。代价：幂等表多存一份响应副本（很小）。
- **租约与领取凭证**：重新领取条件 = `status IN (CREATED, IN_PROGRESS)` 且租约已过期（拣到一半断线可被接手）；过期用惰性判断，不做后台回收。凭证不用 version 顶（version 每次拣货都会变，设备需要追着更新，且会重新暴露已隐藏的内部字段）：租约迁移时新增 `claim_token`，领取时生成、换手才变；提交时校验，被接手后旧设备自动失去提交资格（fencing token）。
- **聚合关系校验分层**：拣货内只校验任务 ↔ 分配行（`order_id`、`sku_id`、`allocated_qty` 三处整数比较，零额外查询）；明细、库存等其他边的正确性由各自的锁读与条件更新继续约束，不做一次全验。
- 实施顺序（2026-10-05 更新）：①⑤ 验证（MySQL + 竞态测试）→ 幂等表 + Idempotency-Key → 任务↔分配行关系校验 → PDA 强制库位/批次扫描 → 成功/失败返回任务快照 → 任务领取 + 租约。**以上已全部落地**。

### 9.5 验证记录（2026-10-05，本地 MySQL 8.0 + Memurai/Redis）

- 全量后端测试（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`）：`go test ./...` 全绿，含迁移往返（000010 上/下/重放，`wms_task` 租约列与 `wms_idempotency` 表断言）。
- 新增竞态测试 `internal/app/pick_race_test.go`（真实事务 + gorm 回调控制交错）：
  - 取消先到：取消持任务行锁未提交时拣货排队，取消提交后拣货被拒（40007），订单/任务/分配行/库存均无拣货写入；
  - 拣货先到：拣货持锁未提交时取消排队，拣货提交后取消看到已开工被拒（50006），订单保持 PICKING、picked_qty=1；
  - 同时到：并发 3 轮恰好一方成功，终态一致（stock = available + allocated，无负库存），无死锁/超时；
  - PDA 集成用例：领取凭证、强制扫码（50010/50011/50009）、幂等重放不重复扣减、同 key 不同内容 409、租约过期可被接手与旧凭证失效（40017/40018/40019）。
- HTTP 端到端（本机后端 + curl）：领取 → 缺库位 400/50010 → 凭证错误 400/40018 → 严格拣货 200 + 任务快照 → 同 key 重放回放同一快照（picked_qty 不重复累加）→ 同 key 不同内容 409/40901。
- k6 波次拣货对比（`scripts/k6/pick-stress.js`，user2 / 仓库92 / SKU433，40 库位 × 50 件 = 2000 件、40 拣货员、CHAOS=0、SCAN_INTERVAL=0；两轮均生成 40 个任务、终态 SHIPPED、无负库存）：

| 指标 | 旧代码（HEAD，整单锁） | 新代码（工作区） | 变化 |
| --- | --- | --- | --- |
| 单次拣货 avg | 207.45ms | 113.59ms | -45% |
| 单次拣货 P95 | 246.86ms | 139.48ms | -44% |
| 单工人迭代耗时（50 次拣货） | 11.84s | 6.46s | -45% |
| 拣货吞吐（pick_ok/s） | 144 | 239 | +66% |
| HTTP 失败率 | 0% | 0% | — |

（对照方式：`git worktree` 检出 HEAD 构建旧后端，与新后端串行运行同一脚本与参数，仅比较 wave 阶段。）

### 9.6 评审修复计划与进展（2026-10-06）

外部评审（用户逐行核对）确认了五项结论并排定优先级，修复按 P0 → P1 → P2 推进：

- P0：① 库存分配翻页与事务冲突重试解耦 ② 分配行/明细/主单补数量上限条件 ③ `IncrDetailPicked` 检查 RowsAffected ④ 审核生成拣货任务时补 `DetailID` ⑤ 拣货补 `t.DetailID == a.DetailID` 校验 ⑥ PDA 强制 `Idempotency-Key` ⑦ 幂等指纹纳入库位/批次/入口类型 ⑧ 幂等结果解析失败返回错误 ⑨ 幂等表 `result_json` 置为非空。
- P1：`LockInventoryByIDs` 前对 ID 排序并加 `ORDER BY id`、收紧"不会死锁"注释；40017/40018/40019 映射 HTTP 409；库位编码缺失 fail fast；补 >5 批库存、同 key 不同扫描、幂等结果损坏等回归测试。
- P2：领取接口幂等、TaskResp 租约字段、设备绑定、快照严格一致读、幂等表清理 Worker、启动 MySQL 后全量重跑。

**已完成（P0-①）**：`inventory/service/stock.go` 删除 `maxAllocateAttempts`（批数不再设上限），改为 keyset 游标翻到底；锁到的行比候选快照少只标记 `staleDetected` 并继续翻页，候选读完仍不足时：有快照变化返回 `Conflict` 交外层 `TxRetry` 换新快照，无变化才报 `AvailableNotEnough`。回归测试 `TestAllocatePagesBeyondBatchWindow`（130 行×1 件、申请 110 件，旧实现必然失败）。已验证：`go test ./internal/modules/inventory/...`、`./internal/modules/outbound/...`、`./internal/app/...` 全绿。

**语义变化（需知悉）**：并发抢空导致"锁到的行 < 快照"时，容量竞争下失败会返回可重试的 40900，而不是立即报"库存不足"——外层换新快照重试后仍不足（且期间无新变化）才会得到 30201。`TestConcurrentAllocateAntiOversell` 的判定已相应放宽（40900 属可重试拒绝），防超卖不变量断言不变。
