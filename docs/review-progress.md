# 持续审查与优化记录

这是一份阶段记录，不是“全项目已完美”的验收报告。当前修改都在本地工作区，未提交、未部署，也未修改线上业务数据。尚未完成所有文件的逐行复核；通过现有检查不代表覆盖全部业务组合。

## 1. 当前判断

| 方面 | 判断与依据 |
| --- | --- |
| Go 与可维护性 | 按业务拆包、普通构造函数、具体 Repository 可以保留。主要问题是错误被误转为不存在、后台工作隐式启动和注释夸大保证；已修正多条实际路径。 |
| 架构 | 单体适合库存、单据和任务共用事务。继续保留 handler/service/repository，允许具体模块有局部差异，不增加通用 CRUD 或微服务。 |
| 数据库 | 有版本化迁移、唯一键、行锁和条件更新基础。但聚合计数、裸表租户条件、收货数量语义曾有实际错误。AutoMigrate 与 SQL 迁移仍需持续对照。 |
| Redis | 条码缓存、单号和演示会话用途不同，不能统一宣传“故障都可降级”。条码已按租户隔离；会话操作原子核对 token；单号失败后改用 UUID，任务号直接复用任务主键，移除业务事务内的 Redis 单号调用。 |
| 并发与 WMS | 已有真实 MySQL 防超卖测试；本轮补充盘点锁读差异、租户写入隔离、收货残品、导入执行权等回归。实物盘点冻结、跨实例文件共享等尚不具备。 |
| Docker | 已有多阶段构建、非 root 用户、持久化卷、服务名连接及健康检查；补充忽略各类 .env 文件。此次环境没有可用 Docker daemon，未验证镜像运行。 |
| 测试 | 后端全量测试强制使用隔离 MySQL 库执行通过，含 000006 迁移回滚与重新应用。Windows 缺少 C 编译器，未能在本机执行 race；CI 已配置。 |
| 安全 | 修复业务路由绕过演示会话、审计租户丢失与脱敏、Token 租户复核、过期权限缓存继续放行。平台旁路、外部 API Key 和演示重置仍需专项复核。 |

## 2. 已修复的关键问题

优先级表示修复前的影响，不表示这些问题目前仍未修复。

| 级别 | 文件 / 位置 | 原问题、后果与处理 |
| --- | --- | --- |
| P0 | `internal/modules/inbound/service/receiving.go` / Receive | 前端 qty 表示包含不良品的收货总量，后端又加上 defective_qty 计算剩余和任务进度，造成拒收或提前收齐。统一为总收货量，残品只在计算上架量时扣除；覆盖混合、全残品和分次收货。 |
| P0 | `internal/modules/stocktake/service/approve.go` / Approve | 差异先普通查询、调整后锁读，可能使用两个账面值；数据库错误还可能被忽略。由 Inventory.Adjust 在锁内返回实际应用差异，单据差异与流水一致，零差异也检查库存。 |
| P0 | `internal/modules/inventory/repository/repository.go` / FIFO 与数量写入 | 裸表和原生 Exec 无法依赖 GORM 模型租户回调。FIFO 显式过滤租户及软删，数量更新改为模型 Updates 保留条件防护；增加跨租户与软删回归。 |
| P0 | `internal/modules/inbound/service/import_worker.go`、`repository/import.go` | 上传与补偿启动独立 goroutine，旧执行可覆盖重跑状态或继续写入。改为受入口管理的单消费者，数据库队列恢复，run_token 校验执行权；每行建单事务再次锁定并核对执行标识。 |
| P0 | `internal/app/router.go` / NewRouter | Gin 注册路由后才 Use(DemoSession)，之前注册的业务接口没有会话校验。先挂载中间件再注册业务路由，使用真实 App 路由做回归。 |
| P0 | `internal/pkg/middleware/middleware.go` / OperLog | Background 丢失请求租户；原始 query 未脱敏；读取失败后将截断请求继续交给业务。保留请求 context、脱敏 query、超限立即拒绝，不让截断内容成为有效请求。 |
| P0 | `internal/pkg/tenant/gorm.go` / Create 回调 | 正租户上下文中显式指定其他 tenant_id 仍可创建。现在返回 ErrMismatch；平台代操作需使用平台上下文。 |
| P0 | `internal/pkg/jwt/jwt.go`、`system/service/auth.go` | 补全 issuer、必要过期字段与有效声明校验，数据库复核租户，避免只验证用户 ID 和版本。 |
| P0 | `internal/modules/system/service/permission.go` / cachedPerms | 权限过期后数据库故障仍回退旧权限，可能无限延长撤销授权。改为查询失败拒绝授权。 |
| P0 | `internal/modules/demo/service/session.go` | GET 后 EXPIRE/DEL 会在过期换主窗口操作其他访客的会话。改为 Lua 比较 token 后原子续期/删除/读取 TTL；重置锁按租户隔离、取得锁后重新核对会话，并限制重置时长。 |
| P0 | `internal/modules/demo/service/service.go` / demoTenantID | 只检查正租户 ID，拥有演示权限的普通租户也可能进入清空数据的路径。会话领取与重置现在只接受配置的演示租户；中间件按已认证租户识别演示身份，不再误伤其他租户的同名用户。 |
| P0 | `internal/modules/system/repository/repository.go` / replaceRoles、权限查询 | 分配角色未核对实际租户，平台代操作还可能生成 tenant_id=0 的错误关联。写入前锁定并核对角色，关联使用用户实际租户；权限联查排除跨租户和软删对象，覆盖历史脏关联与失败回滚。 |
| P0 | `internal/pkg/snowflake/snowflake.go` / Next | 系统时钟回拨时序列可能重置并重用 ID；节点设置未与生成共用锁。改为单个具体生成器和互斥锁，沿用逻辑时间并处理序列耗尽，非法节点启动失败。跨重启仍需要正确节点与时钟配置。 |
| P1 | `internal/modules/system/repository/repository.go` / UpdateUser、DeleteRole | 状态单独更新会清空昵称；删除角色先查后删，期间可能被分配。可选昵称使用指针保留字段存在性；检查引用与删除同事务，和角色分配共用角色行锁，补并发分配回归。 |
| P1 | `internal/pkg/orderno/orderno.go`、`task/service/service.go` | 单号本地计数跨日重置和跨实例降级有冲突风险，任务创建在业务事务中访问 Redis。降级改为已有依赖提供的 UUID，Redis 操作设置超时，任务号复用主键；验证多实例式并发生成、TTL 修复和任务事务回滚。 |
| P1 | `internal/modules/task/service/service.go` / Create、progress | 非法任务被静默跳过，进度相加可能整数溢出。拒绝无效类型、空任务和非正数量，改为剩余量比较；真实 MySQL 并发推进不能超过目标量。 |
| P1 | `internal/modules/system/service/auth.go`、`repository/repository.go` / 登录 | 用户名只在租户内唯一，但登录 First 任意匹配，其他同名账号可能无法登录。增加可选租户编号；省略时检测重名并拒绝，显式 0 仅匹配平台。前端三个入口同步支持，不改变现有唯一用户名账号的登录方式。 |
| P1 | `internal/modules/system/service/login_limiter.go` | 限流检查与失败计数分离，并发请求可同时通过；过期用户名记录长期保留，内存增长无界。锁内预占次数，最多 10000 个记录，请求触发过期清理；补 100 个并发请求、容量及回收测试。 |
| P1 | `web/src/views/login/index.vue` / startDemo | 领取会话失败时已保存 JWT 却没有清理，形成半登录状态。失败时清空认证信息；真实浏览器测试验证 token 不残留。 |
| P0 | `internal/pkg/middleware/api_key.go`、`outbound/service/integration.go` | 外部 API Key 入口原本没有租户绑定，平台旁路可能按编码取到其他租户的仓库/货品，业务单号幂等范围也不明确。现在由服务端配置固定租户，`tenant_id=0` 也使用精确隔离上下文，客户端租户字段不能覆盖；补真实 MySQL API 路由测试。 |
| P1 | `internal/modules/outbound/service/order.go` | 并发相同业务单号插入时，唯一键冲突可能被当成订单号冲突反复重试。插入冲突后先按业务单号回查：已存在返回业务单号重复，由外部入口转为幂等原订单；其他唯一键冲突才继续生成单号。 |
| P1 | `internal/modules/inventory/service/stock.go` / Allocate | 分配改变可用量却没有 ALLOCATE 流水。补充同事务流水，验证上架、分配、释放、发货的总量及可用量连续性。 |
| P1 | `internal/modules/inventory/repository/repository.go` / SummaryBySKU | total 统计库存批次行而不是汇总后的 SKU，导致分页总数错误。改为 distinct sku_id 计数。 |
| P1 | `cmd/wms`、`system/service/audit.go` | 构造阶段启动消费者、退出路径资源管理不完整。分离 run、HTTP 服务关闭和 dotenv 读取；入口启动并等待所有后台消费者。 |
| P1 | `internal/modules/inbound/service/import_parse.go` | 坏文件、空文件可能显示完成，宽松解析接受非法数量。严格整数校验，明确失败状态，限制解压大小并避免内部文件路径暴露。 |
| P1 | basic/inbound/outbound/stocktake/system 的查询入口 | 数据库错误和 context 取消被当作业务不存在。仅 errors.Is(ErrRecordNotFound) 转业务错误，其余保留原因。 |
| P1 | `internal/modules/basic/service/sku.go` | 条码租户内唯一却共用缓存 key，更新部分字段不清缓存。使用租户 key、校验缓存内容，并在更新时使旧 key 失效。 |
| P2 | `internal/pkg/tx/tx.go` | 用错误文本识别 MySQL 类型不可靠。使用 errors.As 读取错误码，保留整事务重试，校验尝试次数。 |
| P2 | `docs/architecture.md`、`docs/database.md` | 原文包含“接口换 RPC 即可拆微服务”“库存一定用 version 乐观锁”“库存不可能对不上”等不实保证。改为描述实际代码与限制。 |
| P2 | `internal/testutil/mysql.go`、库存并发测试 | 全量测试中无上限连接池导致 MySQL 1040 错误；并发分配测试还把任意失败计入库存不足。限制测试连接池，并明确检查失败业务码，避免环境错误掩盖逻辑问题。 |

## 3. 必须保留的边界

- 盘点仍将审核时库存设置成录入的实盘数；这里只修复差异与流水使用不同读值的问题，没有悄悄改为“按创建快照差异累加”。现场作业未冻结时，两种业务口径不同，需要单独设计。
- 部分导入成功仍标为 COMPLETED，同时返回失败行数和摘要；整体文件错误、全部失败、超时或异常中断标为 FAILED。
- 库存 version 递增不代表使用乐观锁；FIFO 依据首次上架时间，同一库存四元组后续收货会合并。
- 权限缓存仍有有效 TTL，多实例没有统一失效广播。本轮只移除了过期后无限回退旧权限的行为。
- 平台 tenant_id=0 仍有跨租户旁路语义；这项兼容性目前保留，不能把“有回调”当成所有 SQL 都隔离的证明。

## 4. 下一阶段的明确顺序

1. 演示场景执行：会话原子续期和释放已修复，继续复核整个演示场景、重置和普通业务在途请求的并发关系。请求开始时校验通过，不等于在整个请求期间持续持有会话；Redis 租约也不能处理任意长时间进程暂停。
2. 平台与身份边界：角色关联、同名账号登录和外部 API Key 租户绑定已修复，继续处理种子账号的跨租户查询和手工平台代操作。明确旁路后再修改，避免迁移、种子或管理员功能被隐式破坏。
3. 业务完整性：任务并发推进已补回归；继续补出库审核与取消、基础数据删除和库存增加并发、盘点作业时间边界的完整链路测试。
4. 文件和部署：导入文件清理、孤立文件、共享存储、应用数据库最小权限、容器启动及关停验证；现有历史数据只先做审计，不自动重算。
5. 完成其余 Go 文件、前端、配置、脚本和文档逐项核对，再输出最终全项目报告。当前不以“lint 零问题”代替这一步。

## 5. 学习时特别注意

| 不推荐写法或理解 | 当前推荐做法 | 原因 |
| --- | --- | --- |
| 所有查询失败都返回 NotFound | errors.Is 分类，其余返回原错 | 断网、取消和不存在的处理不同 |
| 构造函数里直接 go 一个永远循环 | New 只组装，入口运行 Run(ctx) 并等待 | 可以解释资源归属和退出顺序 |
| Context 只是一项形式参数 | 让请求身份、租户、取消真正传到数据库 | 换成 Background 会丢失隔离与取消 |
| 状态等于 PROCESSING 就代表“我的任务” | 本次执行 token + 条件更新 | 状态无法区分旧 worker 与新 worker |
| 有 version 字段就是乐观锁 | 查看 UPDATE 的 WHERE 是否比较旧版本 | 解释应以执行 SQL 为准 |
| 更长目录和更多接口就是更好的架构 | 保留具体模块和必要接口 | 学习成本和维护成本也是设计约束 |
| 普通 string/int 表达所有更新字段 | 可选字段用指针区分省略和零值 | 切换状态不应顺带清空昵称 |
| 检查没有人使用角色，再单独删除 | 角色分配与删除使用同一行锁协议 | 事务边界要覆盖检查和变更 |
| 给每个共享字段单独加原子操作就安全 | 一个 mutex 保护相关状态 | 多字段组合的不变量需要整体同步 |
| 用 First 查找只在租户内唯一的用户名 | 指定租户，或检测歧义后拒绝 | 查到一条不代表身份唯一 |
| 先检查次数，慢操作失败后才增加 | 在同一临界区检查并预占次数 | 并发请求不能共享同一份剩余额度 |

推荐先阅读：入口 → SKU 查询和错误分类 → 收货数量测试 → 库存分配和流水 → 盘点并发测试 → 导入执行权。目标是能说明每个约束为什么存在，而不是背下文件排版。

## 6. 本地验证与发布注意

已执行：强制隔离 MySQL 的 `go test ./... -count=1`，包括真实并发、事务回滚和全量迁移后 000006 回滚/重新应用；演示会话 Lua 测试连接本地 Memurai（Redis 兼容服务），仅创建并清理测试独有的 key；`go build ./...`、`go vet ./...`、goimports 和 golangci-lint。每次 MySQL 测试通过测试辅助函数创建并清理独立 schema。Redis 测试通过 `WMS_TEST_REDIS_ADDR` 显式启用，CI 已配置 Redis 7 服务。

尚未验证：本地 race（缺少 C 编译器）、Docker 构建与运行（daemon 不可用）、本轮改动后的完整浏览器端 E2E。CI 配置不能当作已经实际执行成功的证据。

身份边界这一轮新增 5 个 Playwright 浏览器用例并已本地通过：省略租户、平台租户 0、大整数编号、演示与个人空间传递租户、演示席位失败清除登录态。浏览器用例使用模拟 API，不写业务数据库，不能等同于连接真实后端的完整 E2E。前端 lint、现有 4 个单元测试、类型检查与构建通过；构建使用临时 PATH 指向 Node 24，没有修改系统 Node 安装。

已通过 `docker compose -f deploy/docker-compose.yaml config --quiet` 校验配置展开与语法；此检查不依赖 daemon，也不表示容器已运行成功。

发布前先停止旧版本及导入 worker，运行 000006 迁移，再启动新版本。旧 worker 不认识 run_token，不能混用两种执行方式。历史含残品收货可能已有错误任务进度；需要按原始记录核对，不能直接批量回写线上库存。

Compose 的应用停止宽限期默认改为 45 秒（`WMS_STOP_GRACE_PERIOD` 可覆盖），为 HTTP 退出、导入任务归还和日志排空留出时间。如果增大应用退出超时，应同步调整容器宽限期。

本轮补充：`WMS_INTEGRATION_TENANT_ID` 已同时传入迁移容器和应用容器；仓库状态接口改为只更新 `status`，不再用空名称和备注覆盖已有基础资料。相关修改已通过基础模块测试和 Compose 配置校验。


## 7. 2026-09-22 E2E 与测试辅助修复

- `internal/testutil/mysql.go` 现在在解析测试 DSN 后强制 `ParseTime=true`，并新增 `mysql_test.go` 验证格式化和 round-trip 不丢参数。
- `scripts/windows/verify.ps1 -WithE2E` 不再直接对持久化本地栈执行 Playwright，而是委托 `scripts/e2e.ps1` 启动独立 Compose 项目和独立数据卷。
- 使用临时 MySQL 8.0.46 和 Memurai 在全新数据库上执行 `go test ./... -count=1` 与真实后端 Playwright E2E，结果分别为通过和 15 passed。
- Linux `go test -race ./... -count=1` 已通过 `verify.ps1 -WithRace` 实际验证。
- 独立 Compose E2E 已通过 `scripts/e2e.ps1 -NoBuild`：15 passed，并自动清理测试数据卷。
- 当前剩余动作只有提交、推送和 GitHub Actions 验证。

## 8. 2026-09-22 夜间 heartbeat 第 1 轮

- 已读：任务粘贴文本、`docs/context-handoff.md`、`docs/go-style.md`、`docs/review-progress.md`；已核对真实 Git 状态和最近 5 次提交。
- 基线：`main` 与 `origin/main` 均为 `eff39ba`，开始前工作区干净。
- 修改：`system/model.Versioned.Version` 不再序列化为 API JSON；新增系统用户敏感字段与导入任务 RunToken/文件路径的序列化回归测试。
- 测试证据：`go test ./internal/modules/system/model ./internal/modules/inbound/model -count=1` 通过；`gofmt` 与 `git diff --check` 通过。
- commit：`c2a33f0 fix(model): 隐藏 API 序列化中的内部版本`。
- 待确认决定：无。
- 未解决边界：部分列表/详情接口仍直接返回 GORM Model；本轮只关闭内部版本泄露，后续可在不改变前端契约的前提下逐模块评估必要的 Response DTO。
- 推送：按规则等待 2～3 个绿色 commit 后统一推送并检查 GitHub Actions。

## 夜间自动维护进展（2026-09-22）

- `c2a33f0`：隐藏实体内部 `version`，并补充入库任务、用户认证字段的 JSON 泄漏测试。
- `7f37fa9`：调整模型测试中的敏感样式字面量，消除 gosec G101 误报，不改变测试目标。
- 验证：`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过。
- 已推送 `main`，GitHub Actions run 35750130693 全绿。
- 下一轮：继续 DTO、Model、Response 和前后端 API 契约边界审计。


## 9. 2026-09-23 夜间 heartbeat 第 2 轮

- 基线：`main` 与 `origin/main` 均为 `7f37fa9`，开始前工作区干净。
- 修改：system 用户列表、角色列表和全部角色接口不再直接返回 GORM Model，改用 `UserResp`、`RoleResp`，由 service 显式转换。
- 契约：保留原 JSON 字段、字符串 ID 和 RoleIDs 字符串数组格式；前端类型无需同步修改。
- 测试证据：`go test ./internal/modules/system/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt` 与 `git diff --check` 通过。
- commit：`8007fb3 refactor(system): 隔离用户角色响应模型`。
- 待确认决定：无。
- 未解决边界：其他模块仍有 GORM Model 直接作为 API 响应的路径，后续按模块逐个保持契约迁移。
- 推送：等待下一个绿色 commit，与当前 commit 合并推送并检查 GitHub Actions。

## 夜间自动维护进展（2026-09-23）

- `8007fb3`：system 用户/角色列表改为独立 Response DTO，避免直接序列化 GORM Model；新增敏感字段响应测试。
- `51f0bad`：修复仓库和库位列表筛选契约；后端接收并应用 `status`、`zone`、`keyword`，前端库位编码参数统一为 `keyword`；补充 0 状态绑定和 dry-run SQL 测试。
- 验证：`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、前端 lint/build 均通过。
- 已推送 `main`，GitHub Actions run 35752728145 全绿。
- 下一轮：继续审查 DTO/Response 直接暴露、前后端参数名和响应 data 类型。


## 10. 2026-09-23 夜间 heartbeat 第 3 轮

- 基线：`main` 与 `origin/main` 均为 `51f0bad`，开始前工作区干净。
- 修改：task HTTP 查询不再直接返回 GORM Model，新增 `TaskResp` 和显式转换；内部 `TaskAPI` 仍保留 Model，避免影响 inbound/outbound/demo 的事务内调用。
- 修复：task 详情 handler 不再把所有查询错误统一改成 `TaskNotFound`，数据库和 context 错误交由统一响应层分类。
- 契约：保留任务响应原字段、字符串 ID、`created_at/updated_at`，不暴露内部 `version`。
- 测试证据：`go test ./internal/modules/task/... -count=1` 通过；`gofmt`、`git diff --check` 通过。
- commit：`42d7ebb refactor(task): 隔离任务查询响应模型`。
- 待确认决定：无。
- 未解决边界：只剩一个待推送 commit，等下一次绿色小任务后统一推送并检查 GitHub Actions。

## 11. 2026-09-23 夜间 heartbeat 第 4 轮

- `42d7ebb`：task 查询新增独立 `TaskResp`，HTTP 层不再直接返回 GORM Model；TaskAPI 仍保留 Model，避免影响事务内模块调用。
- `38c7750`：middleware 按请求上下文、认证、恢复、访问日志、操作日志和 CORS 拆分为独立文件，仅移动代码。
- 验证：全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt`、`git diff --check` 均通过。
- 已推送 `main`，GitHub Actions run 35754659654 全绿。
- 下一轮：继续按模块隔离剩余 HTTP Response Model，并核对前端参数和 data 类型。


## 12. 2026-09-23 夜间 heartbeat 第 5 轮

- `2a13c3a`：inventory 列表和库存流水列表改用显式 Response DTO，保留现有 JSON 字段和字符串 ID 契约。
- 测试：新增库存数量字段、流水追踪字段和内部 version 不暴露测试；全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt` 均通过。
- 当前状态：本地有一个待推送 commit，等下一次绿色小任务后合并 push 并检查 GitHub Actions。
- 下一轮：继续评估 basic 或单据查询接口的 Model/Response 边界。


## 13. 2026-09-23 夜间 heartbeat 第 6 轮

- 基线：`main` 与 `origin/main` 均为 `2a13c3a`，工作区干净，有 1 个待推送 commit。
- 修改：basic 仓库、库位、SKU 列表及条码查询改用显式 Response DTO，HTTP 层不再直接返回 GORM Model；原 service Model 方法保留给内部调用。
- 契约：保留原 JSON 字段、字符串 ID 和 `created_at/updated_at`。
- 测试证据：`go test ./internal/modules/basic/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`0b5f842 refactor(basic): 隔离基础资料查询响应模型`。
- 推送：已推送 `38c7750..0b5f842` 到 `main`。
- GitHub Actions：run `35758431079` 在检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。

## 13. 2026-09-23 夜间 heartbeat 第 6 轮

- `3fee4da`：inbound 导入任务查询和列表改用 `ImportTaskResp`，HTTP 层不再直接返回 GORM Model。
- 契约：保留现有 JSON 字段，不暴露 `RunToken` 和文件路径；内部 `GetImport/ListImports` 仍返回 Model，worker 和现有测试不受影响。
- 测试：新增导入任务响应字段和内部路径不暴露测试；全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt` 均通过。
- 当前状态：本地有一个待推送 commit，等下一次绿色小任务后合并 push 并检查 GitHub Actions。
- 下一轮：继续隔离 inbound/outbound/stocktake 单据查询响应，保持原 JSON 契约。


## 14. 2026-09-23 夜间 heartbeat 第 7 轮

- 基线：`main` 与 `origin/main` 均为 `0b5f842` 加 1 个本地 commit `3fee4da`，工作区干净。
- 修改：inbound 入库单列表和创建接口改用显式 `OrderResp`；保留原 `List/Create` Model 方法供导入 worker 等内部调用。
- 契约：保留入库单现有 JSON 字段、字符串 ID、可选 `import_task_id`，不暴露内部 `version`。
- 测试证据：`go test ./internal/modules/inbound/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`b140bbd refactor(inbound): 隔离入库单查询响应模型`。
- 推送：已推送 `0b5f842..b140bbd` 到 `main`（含 `3fee4da`）。
- GitHub Actions：run `35760452506` 在检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。

## 14. 2026-09-23 夜间 heartbeat 第 7 轮

- `5f2ae50`：stocktake 列表、创建和详情接口改用显式 Response DTO，HTTP 层不再直接返回 GORM Model。
- 契约：保留现有 JSON 字段，`actual_qty` 仍为可空指针；内部 Get/List/Create 仍返回 Model，审核和事务流程不变。
- 测试：新增盘点响应字段和内部 version 不暴露测试；全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt` 均通过。
- 当前状态：本地有一个待推送 commit，等下一次绿色小任务后合并 push 并检查 GitHub Actions。
- 下一轮：继续隔离 outbound 查询响应模型。


## 15. 2026-09-23 夜间 heartbeat 第 8 轮

- 基线：`main` 为 `b140bbd`，`origin/main` 为 `b140bbd`，工作区干净，有 1 个本地 commit `5f2ae50`。
- 修改：outbound 出库单列表和创建接口改用显式 `OrderResp`；保留原 `List/Create` Model 方法供集成推送、审核和其他内部调用。
- 契约：保留出库单现有 JSON 字段、字符串 ID、业务单号和数量进度字段，不暴露内部 `version`。
- 测试证据：`go test ./internal/modules/outbound/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`1dc8e0a refactor(outbound): 隔离出库单查询响应模型`。
- 推送：已推送 `b140bbd..1dc8e0a` 到 `main`（含 `5f2ae50`）。
- GitHub Actions：run `35762647831` 在检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：inbound/outbound 详情聚合仍包含 Model，下一轮继续按模块隔离详情 Response。

## 15. 2026-09-23 夜间 heartbeat 第 8 轮

- `19cc340`：system 操作日志查询改用 `OperLogResp`，HTTP 层不再直接返回 GORM Model。
- 契约：保留审计字段和字符串 ID；内部异步写日志仍使用 Model。
- 测试：新增操作日志响应字段与 ID 契约测试；全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt` 均通过。
- 当前状态：本地有一个待推送 commit，等下一次绿色小任务后合并 push 并检查 GitHub Actions。
- 下一轮：检查剩余 Model 响应、前端参数名和 data 类型差异。


## 16. 2026-09-23 夜间 heartbeat 第 9 轮

- 基线：`main` 与 `origin/main` 均为 `1dc8e0a`，工作区干净，有 1 个本地 commit `19cc340`。
- 修改：outbound 详情接口改用显式 `OrderDetailResp`、明细、分配和任务响应结构；保留内部 `Get` Model 聚合供审核、拣货和事务流程使用。
- 契约：保留详情现有 JSON 字段和字符串 ID；明细、分配、任务均不再直接序列化内部 `version`，原 nil 集合仍保持 nil。
- 测试证据：`go test ./internal/modules/outbound/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`747f340 refactor(outbound): 隔离出库详情响应模型`。
- 推送：已推送 `1dc8e0a..747f340` 到 `main`（含 `19cc340`）。
- GitHub Actions：run `35764781380` 检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：inbound 详情聚合仍包含 Model，下一轮继续隔离。

## 16. 2026-09-23 夜间 heartbeat 第 9 轮

- `67358ea`：库存按 SKU 汇总不再返回 `[]map[string]any`，改为 repository `SummaryRow` + `InventorySummaryResp`。
- 契约：保留原有 JSON 字段和字符串 ID，库存三数量模型不变。
- 测试：新增汇总响应数量契约测试；全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt` 均通过。
- 当前状态：本地有一个待推送 commit，等下一次绿色小任务后合并 push 并检查 GitHub Actions。
- 下一轮：检查剩余 API 参数名与响应 data 类型，优先前端契约。


## 17. 2026-09-23 夜间 heartbeat 第 10 轮

- 基线：`main` 与 `origin/main` 均为 `747f340`，工作区干净，有 1 个本地 commit `67358ea`。
- 修改：inbound 详情接口改用显式 `OrderDetailResp`、明细和任务响应结构；保留内部 `Get` Model 聚合供收货、上架和事务流程使用。
- 契约：保留 `order/details/tasks` JSON 字段、字符串 ID 和原 nil 集合行为；任务和明细不再直接序列化内部 `version`。
- 测试证据：`go test ./internal/modules/inbound/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`fc4fe48 refactor(inbound): 隔离入库详情响应模型`。
- 推送：已推送 `747f340..fc4fe48` 到 `main`（含 `67358ea`）。
- GitHub Actions：run `35766921643` 检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：继续审计剩余直接 Model 响应与前后端 data 类型契约。

## 17. 2026-09-23 夜间 heartbeat 第 11 轮

- `93ba301`：demo scenario 按职责拆分，保留 scenario.go 负责编排与公共引用，新增入库/出库/盘点演示文件。
- 测试：全量 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、`gofmt` 均通过。
- 当前状态：本地有一个待推送 commit，等下一次绿色小任务后合并 push 并检查 GitHub Actions。
- 下一轮：评估 demo/concurrent.go 和配置文件的职责边界。


## 18. 2026-09-23 夜间 heartbeat 第 11 轮

- 基线：`main` 与 `origin/main` 均为 `fc4fe48`，工作区干净，有 1 个本地 commit `93ba301`。
- 修改：demo 并发流程按职责拆为出库并发分配、PDA 并发拣货和演示补货三个文件，仅移动代码，保留函数体和业务语义。
- 测试证据：`go test ./internal/modules/demo/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`36fad10 refactor(demo): 按职责拆分并发演示流程`。
- 推送：已推送 `fc4fe48..36fad10` 到 `main`（含 `93ba301`）。
- GitHub Actions：run `35769197878` 检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：继续评估 config 职责拆分和剩余逐模块契约。

## 19. 2026-09-23 夜间 heartbeat 第 12 轮

- 基线：main 为 36fad10，origin/main 为 36fad10，当前有一个未提交的小任务改动。
- 修改：删除 demo Service 中仅做转发的 RunConcurrent，handler 直接调用 RunConcurrentAllocation，让入口函数名准确表达“并发分配”职责。
- 边界：并发演示的 HTTP 路径、请求字段、响应结构、库存和事务流程均未改变。
- 测试证据：go test ./internal/modules/demo/... -count=1、gofmt、git diff --check 均通过。
- commit：d5ae8c9 refactor(demo): 明确并发分配入口命名。
- 当前状态：本地有一个待推送 commit；按每 2～3 个绿色 commit 统一推送，下一轮继续审计剩余 Model/API 契约。
- 待确认决定：无。
- 未解决边界：demo.ActivitySnapshot 等剩余响应仍可能直接携带领域 Model，下一轮逐项确认是否影响前端契约和学习阅读。

## 19. 2026-09-23 夜间 heartbeat 第 12 轮

- 基线：`main` 与 `origin/main` 均为 `36fad10`，工作区干净，有 1 个本地 commit `d5ae8c9`。
- 修改：config 配置结构、账号映射与加载/默认值/校验逻辑拆分为 `types.go` 和 `config.go`，仅移动代码，不改配置语义。
- 测试证据：`go test ./internal/pkg/config/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；`gofmt`、`git diff --check` 通过。
- commit：`91b2fc4 refactor(config): 拆分配置结构与加载职责`。
- 推送：已推送 `36fad10..91b2fc4` 到 `main`（含 `d5ae8c9`）。
- GitHub Actions：run `35771364003` 检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：继续审计剩余 API 参数名、响应 data 类型和逐模块边界。

## 20. 2026-09-23 夜间 heartbeat 第 13 轮

- 基线：main 与 origin/main 均为 91b2fc4，工作区干净。
- 修改：demo 操作记录接口不再直接返回 GORM Model，新增显式 ActivitySnapshot、操作日志、入库单、出库单、盘点单、任务和库存流水响应结构；查询先读取 Model，再由 response helper 映射。
- 契约：保留现有公开字段、字符串 ID、状态枚举和 nil/空集合行为；继续隐藏内部 ersion，响应结构不再随 Model 字段无意识变化。
- 测试证据：新增 TestActivitySnapshotKeepsBusinessRecordsAndHidesInternalVersion；go test ./... -count=1、go vet ./...、golangci-lint v2.13.2、gofmt、git diff --check 均通过。
- commit：c61bde9 refactor(demo): 隔离操作记录响应模型。
- 当前状态：本地有一个待推送 commit；按每 2～3 个绿色 commit 统一推送，下一轮继续检查剩余 Model/API 契约。
- 待确认决定：无。
- 未解决边界：继续审计其他直接返回 Model 的 handler 和前端 data 类型差异。

## 20. 2026-09-23 夜间 heartbeat 第 13 轮

- 基线：`main` 与 `origin/main` 均为 `91b2fc4`，工作区干净，有 1 个本地 commit `c61bde9`。
- 修改：批量创建库位响应从 `gin.H{"created": n}` 改为显式 `LocationBatchResp`，保持前端 `{created:number}` 契约不变。
- 测试证据：`go test ./internal/modules/basic/... -count=1`、`go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2` 均通过；新增响应 JSON 契约测试。
- commit：`c88dce8 refactor(basic): 明确批量库位响应结构`。
- 推送：已推送 `91b2fc4..c88dce8` 到 `main`（含 `c61bde9`）。
- GitHub Actions：run `35773560060` 检查时仍为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：继续审计剩余 API 参数名、响应 data 类型和逐模块边界。

## 21. 2026-09-23 夜间 heartbeat 第 14 轮

- 基线：main 与 origin/main 均为 c88dce8，工作区干净。
- 修改：将 bootstrap 的种子逻辑拆分为 seed.go（只保留 Seed 编排）、seed_admin.go（内置管理员/角色）和 seed_demo.go（演示基础资料与演示单据）；只移动代码，不改变事务、幂等和业务语义。
- 原因：管理员种子与演示业务种子职责不同，分开后阅读和修改边界更清楚；没有新增目录或抽象。
- 测试证据：go test ./internal/bootstrap/... -count=1、go test ./... -count=1、go vet ./...、golangci-lint v2.13.2、gofmt、git diff --check 均通过。
- commit：9220250 refactor(bootstrap): 拆分管理员与演示数据种子。
- 当前状态：本地有一个待推送 commit；按每 2～3 个绿色 commit 统一推送，下一轮继续逐模块阅读和安全的命名/职责审查。
- 待确认决定：无。
- 未解决边界：继续检查剩余大文件的职责边界和真正影响阅读的命名。

## 21. 2026-09-23 夜间 heartbeat 第 14 轮

- 基线：`main` 为 `9220250`，`origin/main` 为 `c88dce8`，工作区干净。
- 本轮未新增代码改动：继续审计 handler 响应、错误分类、后台 context 和大文件职责后，未发现值得安全修改的小任务，避免制造重构。
- 验证证据：对现有 `9220250` 重新执行 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`，均通过；`git diff --check` 通过。
- 推送：已推送 `c88dce8..9220250` 到 `main`。
- GitHub Actions：run `35775645649` 检查期间为 Queued/In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：若后续仍无明确安全问题，不继续为拆分而拆分；转入逐模块只读验收和测试缺口核对。

## 22. 2026-09-23 夜间 heartbeat 第 15 轮

- 基线：main 与 origin/main 均为 9220250，工作区干净。
- 修改：在 inventory/service/stock.go 增加三数量库存不变量说明，并把手写循环中的 tupleRetry / i 改为 tupleCreateRetries / attempt，表达这是库存四元组并发创建的有限重试。
- 边界：只改注释和局部命名，不改变库存公式、事务边界、锁顺序、流水字段或错误语义。
- 测试证据：go test ./internal/modules/inventory/... -count=1、go test ./... -count=1、go vet ./...、golangci-lint v2.13.2、gofmt、git diff --check 均通过。
- commit：911579f docs(inventory): 说明三数量库存不变量。
- 当前状态：本地有一个待推送 commit；按每 2～3 个绿色 commit 统一推送，下一轮继续逐模块业务不变量阅读和测试缺口核对。
- 待确认决定：无。
- 未解决边界：库存并发和流水测试已覆盖主要路径；后续只补充能实际解释业务行为的测试，不机械增加重复用例。

## 22. 2026-09-23 夜间 heartbeat 第 15 轮

- 基线：`main` 为 `911579f`，`origin/main` 为 `9220250`，工作区干净。
- 本轮未新增代码改动：继续审计 handler 响应、错误分类、context 和资源归属后，无新的安全高价值任务，避免制造重构。
- 验证证据：对现有 `911579f` 执行 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`，均通过；`git diff --check` 通过。
- 推送：已推送 `9220250..911579f` 到 `main`。
- GitHub Actions：run `35777714803` 检查期间为 In progress，可见 job 未发现失败；待后续轮次确认终态。
- 待确认决定：无。
- 未解决边界：后续转入逐模块只读验收和测试缺口核对；没有明确问题时不新增改动。

## 23. 2026-09-23 夜间 heartbeat 第 16 轮

- 基线：main 与 origin/main 均为 911579f，工作区干净。
- 修改：InboundOrderItem.import_task_id 从前端可选 string 改为 string | null，并在类型旁说明手工入库为 null、Excel 导入单为批次任务 ID，和 Go 端 *string 的 JSON 契约一致。
- 边界：这是前端类型契约修正，不改变 API 路径、请求参数、响应字段和业务逻辑。
- 验证证据：
pm run lint、
pm run build、git diff --check 均通过。
- 已知环境边界：本机 Node 为 20.18.1，Vite 提示要求 20.19+ 或 22.12+；构建仍成功，但版本要求需要在后续环境/文档核对中确认，不能声称已消除该警告。
- commit：4103101 fix(web): 对齐入库任务 ID 的可空契约。
- 当前状态：本地有一个待推送 commit；按每 2～3 个绿色 commit 统一推送，下一轮继续 API 契约与逐模块阅读。
- 待确认决定：无。
- 未解决边界：继续检查 Go JSON 字段与前端 TS 类型中可能为 null 的字段。

## 24. 2026-09-23 夜间 heartbeat 第 16 轮

- 基线：`main` 为 `4103101`，`origin/main` 为 `911579f`，工作区干净。
- 本轮未新增代码改动：可空响应字段审计只发现 `import_task_id` 和 `actual_qty`，前端类型均已正确表达 `null`，无需修改。
- 验证证据：对现有 `4103101` 执行 `go test ./... -count=1`、`go vet ./...`、`golangci-lint v2.13.2`、前端 `npm run lint`、前端 `npm run build`，均通过；`git diff --check` 通过。
- 已知环境边界：本机 Node `20.18.1` 低于 Vite 提示要求 `20.19+` 或 `22.12+`；构建成功，但该警告仍需在环境/文档核对中保留，不能写成已消除。
- 推送：已推送 `911579f..4103101` 到 `main`。
- GitHub Actions：run `35779941680` 检查期间为 In progress，可见 job 未发现失败；Pages run `35779939833` 已成功。
- 待确认决定：无。
- 未解决边界：后续继续只读逐模块验收；没有明确问题时不新增改动。

## 25. 2026-09-23 夜间 heartbeat 第 17 轮（只读收尾审计）

- 基线：main 与 origin/main 均为 4103101，工作区干净；GitHub Actions CI run 对应 4103101 已成功，Pages 已成功，Deploy run 仍在进行中。
- 只读审计范围：cmd/app/bootstrap/pkg、system/basic/inventory/task、inbound/outbound/stocktake、demo/ai 的 handler 响应、context 参数、 goroutine/ticker 生命周期、事务/状态机注释和测试缺口。
- 结论：HTTP handler 已通过 Service/Response DTO 输出，未发现需要安全修复的直接 Model 响应或资源释放问题；没有为了制造改动而新增重构。
- 本地环境边界：Node 20.18.1 低于 Assert-Node 要求的 20.19+，CGO_ENABLED=0 且无 gcc，不能在本机执行 erify.ps1 -WithRace；本地 Docker Compose 服务也未运行，因此未宣称本地 race/E2E 已执行。GitHub Actions 的 CI 已在 Linux + MySQL + Redis 环境运行 race 测试并成功。
- 待确认决定：无。
- 未解决边界：若需要在用户机器上复现最终 erify.ps1 -WithRace / 2e.ps1，需先升级 Node 并启动 Compose/安装可用的 CGO 工具链；这不影响当前已推送代码的 CI 结果。

## 26. 2026-09-23 夜间 heartbeat 第 18 轮（只读收尾审计）

- 基线：`main` 与 `origin/main` 均为 `4103101`，工作区干净。
- 只读检查：Go 响应的可空指针字段、handler 错误分类、后台 context/ticker、资源释放和前端 API 类型映射。
- 结论：可空响应字段仅 `import_task_id` 与 `actual_qty`，前端均已正确使用 `null`；未发现需要安全修改的新问题，未制造重构或重复测试。
- CI：`4103101` 对应 GitHub Actions run `35779941680` 与 Pages run `35779939833` 均为 Success。
- 待确认决定：无。
- 未解决边界：本机 Node 20.18.1 仍低于 Vite 要求的 20.19+；本地构建成功，但该环境版本事项保留为已知边界。

## 27. 2026-09-23 夜间 heartbeat 第 18 轮（最终收尾）

- 基线：main 与 origin/main 均为 4103101，工作区干净。
- CI：4103101 对应 CI、Pages 和 Deploy 三个 GitHub Actions 均为 Success。
- 最终结论：本夜维护范围内没有新的安全高价值代码任务；本地 Node/CGO 边界已记录，不能用本地 erify.ps1 -WithRace 结果替代已成功的 Linux CI race 测试。
- 自动化：夜间 go heartbeat 已完成预定维护目标，现停止，避免继续为空转或制造无意义改动。

## 27. 2026-09-23 夜间 heartbeat 第 18 轮（只读验证）

- 基线：`main` 与 `origin/main` 均为 `4103101`，工作区干净；`git diff --stat`、`git diff --check` 均无输出。
- 只读审计：继续核对 handler 响应 DTO、可空字段、错误分类、context/goroutine 生命周期和测试覆盖；未发现需要安全修改的新问题。
- CI：`4103101` 对应 GitHub Actions CI run `35779941680` 与 Pages run `35779939833` 均为 Success。
- 待确认决定：无。
- 未解决边界：本机 Node 20.18.1 仍低于 Vite 要求的 20.19+；该环境事项保留，不在本轮为消除警告而调整依赖或版本声明。

## 28. 2026-09-23 本地 race 与真实后端 E2E

- 环境：Node 已切换为 `22.12.0`，npm `10.9.0`；Docker Engine `29.8.0`，Compose `5.5.1`。
- 修复：`scripts/windows/verify.ps1` race DSN 中 `$database?charset` 的 PowerShell 变量边界问题，改为 `${database}`。
- 本地栈：启动 `deploy-mysql-1`、`deploy-redis-1` 后均达到 healthy；验证结束已用 `docker compose down --remove-orphans` 停回原状态，数据卷保留。
- 验证证据：`scripts/windows/verify.ps1 -WithRace` 全部通过，包括 Linux `go test -race ./... -count=1`、全量测试、vet、lint、前端 lint/单测/build。
- E2E：`scripts/e2e.ps1` 使用独立 Compose 项目和数据卷，真实后端 Playwright `15 passed`，测试后自动删除独立容器、网络和数据卷。
- commit：`4c0d1be fix(scripts): 修正 race 验证数据库 DSN`。
- 推送：已推送 `4103101..4c0d1be` 到 `main`。
- GitHub Actions：run `35800186883` 已触发，检查时仍为 In progress 且未发现失败。
