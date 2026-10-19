# 本地开发与验证

[文档导航](README.md) · [项目首页](../README.md)

以下命令均从仓库根目录执行，另有说明的除外。下方按日期保留历史验证记录，其中旧阶段的“未提交”“幂等尚待实施”只代表当时状态；当前幂等已完成，以 2026-10-18 验收记录为准。

## 2026-10-19 验证门禁 D02 / D03

基线 `main` / `926ad0e`。本轮只修复本地集成测试和 CI 漏洞扫描的假通过问题；没有修改业务运行逻辑、数据库迁移或前端功能。原有外部单保护、迁移 000012、Makefile/Windows/k6 未提交改动保留。verify.ps1 原有本地状态仅为行尾差异，本轮仅提交新增门禁改动。

- D02：`verify.ps1` 调用新的 `test-go.ps1`，默认要求显式测试 DSN/Redis 地址，临时强制 WMS_TEST_REQUIRED=1；JSON 事件区分无测试文件的包与实际测试跳过。缺少配置、测试失败、跳过用例、没有成功测试包、无效 JSON 均不能通过；成功和失败均恢复环境。显式 AllowSkippedIntegration 只能产生部分检查结果，不显示完整通过。
- D03：`scripts/check-vulns` 以子进程运行真实 govulncheck，检查退出状态和完整 JSON 对象流、协议、符号级源码扫描及 SBOM 目标。扫描失败/取消、空/截断/损坏/无目标报告均拒绝；保留已有漏洞策略、去重输出与原始报告，CI 上传 artifact。报告输出路径为本地 CLI 参数、使用 0600 文件权限，单行 gosec 说明限定于该开发者输出边界，不关闭全局规则。
- 验证完整脚本时发现系统级数量审计被既有租户字符串检查误报；仅补充其跨全部 tenant_id 的意图注释，SQL 与运行逻辑不变。D07 模块依赖护栏仍待实施。

从 index 导出独立源码快照 `tmpgen/gates-20261019/source`（源码树 `d2381cb76afea1fcfeb058501f4e6ea00b5dd193`，之后仅补文档/报告忽略规则）执行：

```powershell
$env:WMS_TEST_DSN='root@tcp(127.0.0.1:23306)/?parseTime=true&timeout=2s'
$env:WMS_TEST_REDIS_ADDR='127.0.0.1:26379'
.\scripts\windows\verify.ps1
.\scripts\tests\verification-gates.tests.ps1
go test ./scripts/check-vulns -count=1
go run ./scripts/check-vulns -scanner <本机 govulncheck 路径> -report vuln.json
```

| 验证 | 结果 |
| --- | --- |
| 完整 verify 入口 | 通过：gofmt、Go build/真实 MySQL 与 Redis 全量测试（跳过 0 项）、vet、golangci-lint 0 issues、租户原生查询检查、前端 lint/21 文件 194 单测/vue-tsc/build、diff 检查 |
| PowerShell 门禁回归与语法检查 | 通过：缺 DSN/Redis、失败退出、实际跳过、空/损坏 JSON、无测试文件包、显式部分模式、环境恢复；无 Pester 依赖，已纳入 CI |
| 扫描器进程与报告回归 | 通过：扫描失败/部分输出后失败、缺失扫描器、取消、空/损坏/协议或级别不符/缺少目标报告；可修复已调用阻断、无修复仅告警、模块/导入未调用不升级、重复 finding 去重 |
| 真实扫描（govulncheck v1.8.0，Go 1.26.9） | 完成且门禁通过：无已调用且已有修复版本的漏洞，无修复版本告警 0；仅为本次扫描结论 |
| 真实扫描失败目标 | 无匹配包时扫描器退出 2，门禁退出 1，未输出成功；保留原始报告 |
| CI YAML | 本地解析通过，新增脚本回归及报告上传步骤结构有效；不代表远端已执行完成 |

仅使用本轮启动的本地测试 MySQL 23306 / Redis 26379，各 Go fixture 自建临时 schema；没有连接开发或生产业务库。测试服务在结束后关闭。未运行本地 race、E2E、k6 或生产部署；本快照没有 .env，完整脚本未执行条件式 Docker Compose 配置检查。本轮前端无功能变更，前端检查由完整入口一并执行。

## 2026-10-18 十项 P2 专项

基线 `main` / `e67b5ea`。修复问题总表 F7/F9/F12/F13/F14/F15/F16/F20/F21/F22，并将当前登记未关闭 P1/P2 均更新为 0；工程债务和未完成全仓审查不随之关闭。实现与回归逐项见 review-findings.md 的 P2 专项表。

主要改动：system 的登录/权限/密码；前端 auth/request 和密码规则；task API 与入出库详情聚合、业务选项；五个业务模型的 27 个数量字段、DTO/累计校验、bootstrap 开发数量审计；AI 聚合查询、导入文件补偿、外部单 SKU 身份核对及库存不足口径。新增或扩展 `p2_auth_test.go`、`task_pagination_test.go`、`order_quantity_test.go`、`external_identity_test.go`、`low_stock_test.go`、`import_completed_cleanup_test.go`、`availability_test.go`、bootstrap/migrations 的 `quantity_test.go`、实盘 HTTP 校验，以及前端 request/options/quantity 回归和 P2 选项 E2E。

本轮仅提取 P2 改动进入 index；原有外部单来源/查询/取消/删除保护、迁移 000012、Makefile 和 Windows/k6 脚本保留本地。四个共享出库文件按原工作区与 HEAD 对齐提取增量；不能把工作区整体通过当成拟发布提交通过。实际从 index 导出的独立源码快照（源码树 `5deab6160e8f8befdc51fcf8b7c158960017751b`，后续仅补文档）验证：

```powershell
$env:WMS_TEST_REQUIRED='1'
$env:WMS_TEST_DSN='root@tcp(127.0.0.1:23306)/?parseTime=true&timeout=2s'
$env:WMS_TEST_REDIS_ADDR='127.0.0.1:26379'
go test ./... -count=1
go vet ./...
go build ./...
golangci-lint run ./...
# web/
npm run lint
npm run test
npm run build
$env:E2E_BASE_URL='http://127.0.0.1:25173'
npx playwright test p2-options.spec.ts p1-order-safety.spec.ts idempotency-recovery.spec.ts inventory-lifecycle.spec.ts --reporter=list
```

Go 全量真实 MySQL/Redis 测试、vet/build 和全仓 lint（0 issues）通过；gofmt/goimports 与 diff 空白检查通过。前端 lint、21 文件/194 单测、vue-tsc/build 通过；真实后端 Playwright 12 场景通过，包含新增超过 100 条选项后选择仓库/SKU并真实建单、P1 四场景、幂等恢复三场景及库存闭环四场景。测试依赖为独立本地 MySQL 23306/Redis 26379；E2E 使用专用 `gowms_p2_e2e_20261018` schema、API 28080 与 Vite 25173，没有连接开发或生产业务库。

数量选择保持正式 SQL 的 INT 契约，本轮不改旧迁移、不新增生产收窄迁移、不裁剪历史数据；开发 AutoMigrate 必须在停止其他写入后运行，审计超限值会阻止改表。限流/权限缓存仍是进程内能力，分页聚合不是严格一致读快照。未运行本地 race（CGO_ENABLED=0）、k6、生产部署或全部其它 E2E；远端 CI 状态另行核对，不能以本地通过代替远端结果。

## 2026-10-18 七项 P1 专项

基线 `main` / `f0d7bca`，范围为问题总表 F1/F2/F4/F5/F11/F17/F18。对应实现与回归已补齐；提交说明与 GitHub 推送说明使用中文。当前工作区原有外部集成来源/查询/取消/删除保护、迁移 000012、Makefile 和 Windows/k6 脚本改动保留，不纳入本轮发布；共享的 `outbound/service/order.go` 只提取 P1 主数据锁改动进入暂存区。

### 实现与回归范围

| 问题 | 实现 | 回归 |
| --- | --- | --- |
| F11 | RecordActualReq.ActualQty 为 required/min=0 指针，HTTP 绑定通过后才解引用 | `stocktake/handler/actual_test.go`：真实 HTTP + MySQL，缺失/null/负数不改明细且审核拒绝，显式 0/正数成功 |
| F18 | 正 INT32 坐标，先除法检查总量≤1000，再按数量迭代，前端同步端点上限 | `basic/dto/dto_test.go`、`basic/service/location_limits_test.go`：1000/1001、零/负/反向、极大维度、MaxInt 和最大合法终点，幂等重放 |
| F4 | 清空旧表单、加载代次与 ready/fetching 状态、禁止加载未完成保存、保存中防双击 | `useInboundOrderForm.spec.ts` 四个异步场景与 `p1-order-safety.spec.ts` 的真实后端编辑场景 |
| F5 | Layout 按 route.path 重建 router-view，单据 ID 变化重新初始化 | `p1-order-safety.spec.ts` 三个真实 SPA Router 跳转场景，入库/出库提交、盘点录入/审核均只作用 B，A 不变 |
| F1/F2 | 调用方事务内校验仓库、共享锁批量读取 SKU 快照；仓库→SKU 升序共享锁与删除排他锁互斥，允许并发建单 | `internal/app/order_references_test.go`：单连接池编辑/失败回滚、入出库创建与 SKU 删除两种交错、租户隔离；原外部同号并发回放继续回归 |
| F17 | 新上架写库存前按仓库→库位锁顺序核对当前库位可用性/归属；已成功 key 继续提前回放 | `inbound/service/putaway_location_race_test.go`：禁用先提交拒绝无副作用、上架先持锁则禁用最后生效、禁用后原 key 回放成功 |

仅连接本地专用 MySQL 127.0.0.1:23306 与 Memurai 127.0.0.1:26379，后端集成测试由 OpenIsolatedMySQL 创建回收独立 schema。浏览器用独立测试库；没有连接开发库或生产库。发布前已从暂存区导出源码快照，排除其他本地功能后执行全量校验；结果见本节下表。

### 发布前校验结果

以下检查针对从 Git 暂存区导出的独立源码快照 `tmpgen/p1-release-20261018/source`，不包含外部单保护等其他本地改动。

| 命令/场景 | 结果 |
| --- | --- |
| Go 新增/修改文件 gofmt/goimports；`git diff --cached --check` | 通过 |
| `WMS_TEST_REQUIRED=1 WMS_TEST_DSN=... WMS_TEST_REDIS_ADDR=127.0.0.1:26379 go test ./... -count=1` | 全量通过，真实 MySQL/Redis；含本轮新增 HTTP、单连接池、主数据并发和回滚回归 |
| `go vet ./...`、`go build ./...` | 通过 |
| `golangci-lint run ./...` | 通过，0 issues |
| web：`npm run lint`、`npm run test -- --reporter=dot`、`npm run build` | 通过；18 文件、177 单测；build 含 vue-tsc |
| web：`E2E_BASE_URL=http://127.0.0.1:25173 npm run test:e2e -- p1-order-safety.spec.ts idempotency-recovery.spec.ts inventory-lifecycle.spec.ts --reporter=line` | 11 passed；4 个 P1 编辑/真实 SPA 路由回归、3 个幂等恢复、4 个原业务闭环 |

E2E 使用独立 `gowms_p1_release_e2e_20261018` schema，前后端均由这份提交快照启动（API 28080、Vite 25173）。未运行本地 race（CGO_ENABLED=0）、k6 或生产部署；数据库数量字段类型、其他 P2 与工程债仍按原范围保留。

## 2026-10-18 幂等专项：已完成并通过本地验收

基线：`main` / `ca24141`；`git ls-remote origin refs/heads/main` 已核对 GitHub main 同一提交。依据 `docs/review-findings.md` 的 F3/F6/F10，含相关 F8 与 F1 领取分支，本次提交包含本轮幂等实现、测试及文档。既有外部单保护、迁移 000012、Makefile/Windows 脚本/k6 修改完整保留，不属于本轮新增实现。

### 修复及文件范围

- 后端：`internal/modules/inbound/service/{receiving,putaway}.go`、`internal/modules/stocktake/service/approve.go`、`internal/modules/outbound/service/pick.go`、`internal/pkg/errcode/errcode.go`。五条 reconcile 读取失败/取消返回 50301 / HTTP 503，不能退回确定拒绝促使客户端丢 key；领取冲突诊断复用当前事务的锁读，避免再借连接。没有变更库存计算、业务指纹或迁移。
- 前端：`web/src/composables/inbound/{useReceive,usePutaway}.ts`、`outbound/usePdaPick.ts`、`stocktake/useStocktakeApprove.ts`；恢复复用普通提交校验，发送中不能放弃，提交前检查存储读取，状态按账号/演示会话隔离，旧身份晚返回保留原 pending；storage 监听随组件/effect scope 销毁注销。
- 入口：`web/src/components/{ReceiveDialog,PutawayDialog,PickDialog}.vue` 与三个业务详情页、盘点列表。修正 storageError 嵌套 Ref；终态恢复入口独立于新作业入口，上架/拣货可选择有 pending 的已完成任务并优先选中待确认项。
- 新增回归：`internal/app/{claim_pool,reconcile_failure}_test.go`、入库/盘点 `service/reconcile_failure_test.go`、`web/src/composables/operationRecovery.spec.ts`、`web/tests/e2e/idempotency-recovery.spec.ts`。已有 `internal/app/idempotency_race_test.go` 的屏障改在 SELECT 完成且未命中后通知，Insert 失败回滚用例改为拣满请求以实际经过发货库存/流水分支。

### 实际环境与结果

使用独立 MySQL 8.0.46（127.0.0.1:23306）、Memurai（127.0.0.1:26379），MySQL data dir 为 `tmpgen/idem-20261018/mysql`，没有连接开发库或生产库。后端测试由 OpenIsolatedMySQL 创建/回收临时 schema；浏览器用独立 `gowms_idem_e2e_20261018` schema，当前源码编译的后端在 28080、Vite 在 25173。未使用业务响应 mock 证明并发或库存正确性。

| 命令 | 结果 |
| --- | --- |
| gofmt/goimports（本轮 Go 文件） | 通过 |
| `WMS_TEST_REQUIRED=1 WMS_TEST_DSN=... WMS_TEST_REDIS_ADDR=127.0.0.1:26379 go test ./... -count=1` | 通过；真实 MySQL/Redis；含五命令核对失败故障注入、领取单连接池、并发与回滚回归 |
| `go vet ./...` / `go build ./...` | 通过 |
| `golangci-lint run ./...` | 1 条既有问题：本轮开始前已存在的未跟踪 `internal/pkg/errcode/integration.go:3` 导出变量注释不以 ExternalOrderProtected 开头（ST1022）；未改该外部单保护文件 |
| `npm run lint` / `npm run test -- --reporter=dot` / `npm run build`（web） | 通过；17 文件、173 单测；build 含 vue-tsc |
| `E2E_BASE_URL=http://127.0.0.1:25173 npm run test:e2e -- idempotency-recovery.spec.ts inventory-lifecycle.spec.ts --reporter=line`（web） | 7 passed；3 个新恢复场景覆盖收货/上架/领取/拣货/盘点审核，另 4 个原有入库导入/入库/出库/盘点闭环回归 |
| `git diff --check` | 通过 |

E2E 故障注入先通过 route.fetch 把请求发送到真实后端，确认响应 code=0 后丢弃响应，刷新页面，从终态/任务入口恢复并断言 key 与原业务参数相同，库存/任务/单据数量不重复累加。拣货的 claim_token 不属于业务指纹，刷新后恢复无须重建旧凭证。

发布前再次从 Git 暂存区导出独立源码快照（`tmpgen/idem-release-20261018/source`），不包含其他未提交功能；该快照的 `go test ./... -count=1`（强制真实 MySQL/Redis）、`go vet ./...`、`go build ./...`、`golangci-lint run ./...` 均通过（0 issues），前端 lint、173 单测与 build 通过。上表 ST1022 仅存在于原工作区的另一个未提交文件，不属于本次提交，也没有通过放宽 lint 规则绕过。E2E 沿用本节已完成的 7 场景真实后端验证。

未运行 Linux race（当前 CGO_ENABLED=0、Docker daemon 未启动）、k6、全套其它 E2E 或生产部署。保留 7 天同 key 窗口；不承诺跨 key、跨标签页互斥、清缓存/跨设备后的自动恢复。F1 入库编辑、F2/F4/F5/F7/F9 等其他专项未处理。

## 本地启动

### 1. 环境要求

- Go 1.26+
- Node.js 20.19+ 或 22.12+
- MySQL 8.0.16+
- Redis 7（缓存和单号可降级，演示会话需要 Redis）

### 2. 启动后端

```bash
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS gowms DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
go run ./cmd/wms
```

> 数据库名沿用历史名称 `gowms`（仅内部标识符，与项目显示名 WMS 无关）。Docker 部署可通过 `.env` 的 `MYSQL_DATABASE` 修改；本地开发需同步修改 `configs/config.yaml` 中的 DSN。

服务默认监听 `http://127.0.0.1:8080`。`debug` 模式首次启动会通过 AutoMigrate 建表并初始化管理员；需要验证版本化表结构时先执行迁移；`release` 必须先迁移并初始化管理员：

```bash
go run ./cmd/migrate up              # 只做数据库结构迁移
go run ./cmd/migrate bootstrap-admin # 创建/修复平台管理员（release 必须提供 WMS_ADMIN_PASSWORD）
```

仅在 debug 开发或测试环境需要演示种子时执行：

```bash
go run ./cmd/migrate seed-demo
```

```text
用户名：admin
密码：debug 模式默认 admin123（可用 WMS_ADMIN_PASSWORD 覆盖）；release 模式取 WMS_ADMIN_PASSWORD，未设置会初始化失败
```

首次登录后请立即修改密码。

### 3. 启动前端

```bash
cd web
npm ci
npm run dev
```

访问 `http://127.0.0.1:5173`。开发服务器会将 `/api` 代理到 `http://127.0.0.1:8080`。

### 4. 提交前完整验证

```powershell
# 配置你自己的本地/专用测试实例，以下地址与密码仅作示例；禁止使用生产实例。
$env:WMS_TEST_DSN = 'test_admin:your-test-password@tcp(127.0.0.1:23306)/?parseTime=true&timeout=2s'
$env:WMS_TEST_REDIS_ADDR = '127.0.0.1:26379'
.\scripts\windows\verify.ps1
```

默认通过 `test-go.ps1` 强制 `WMS_TEST_REQUIRED=1`，结束后恢复调用前的环境变量。配置缺失、依赖不可用、Go 失败或测试用例跳过均阻断；Go 的“没有测试文件”包不会误算作测试跳过。只想在缺少依赖时做部分检查，可显式运行 `verify.ps1 -AllowSkippedIntegration`，脚本会报告实际跳过数，并始终标记不能作为完整验证通过。后端单独使用同样门禁可运行 `scripts/windows/test-go.ps1`。

需要同时运行 Linux race 检查时：

```powershell
.\scripts\windows\verify.ps1 -WithRace
```

`verify.ps1` 和 `e2e.ps1` 会检查 Node.js 版本；要求为 20.19+ 或 22.12+。

如果需要额外运行真实后端 Playwright E2E：

```powershell
.\scripts\windows\verify.ps1 -WithE2E
```

### 5. 生成交付压缩包

不要手动压缩整个工作目录，以免把 `.env`、`.git` 或测试产物带入交付包。使用标准脚本：

```powershell
.\scripts\windows\package.ps1
```

脚本要求工作区干净，并使用 `git archive` 只打包当前提交中的跟踪文件；默认输出到 `dist/wms-<commit>.zip`。

## 漏洞扫描门禁

CI 安装 govulncheck 后使用 `go run ./scripts/check-vulns -scanner <扫描器路径> -report vuln.json`。该入口先执行符号级源码扫描，扫描器失败（含加载/网络/取消）、报告为空或 JSON 损坏、协议/扫描级别不符、缺少目标记录均失败。原始报告保留，CI 使用 `go-vulnerability-report` artifact 上传。

策略保持原样：实际调用到且已有修复版本的漏洞阻断；实际调用到但上游未提供修复版本的漏洞告警；仅出现于模块/导入层面的 finding 不升级为实际调用。通过表示本次扫描完成且没有触发上述阻断条件，不等于所有依赖永久无漏洞。CLI 的 `-report` 是开发者/CI 指定的本地输出位置，不接受业务 HTTP 输入。

门禁回归无需 MySQL/Redis：`go test ./scripts/check-vulns -count=1` 和 `scripts/tests/verification-gates.tests.ps1`；后者无需 Pester，验证真实 PowerShell 入口的配置、退出码、跳过与环境恢复规则。CI 同时执行这两类回归。

## 测试与检查

后端：

```bash
go test ./...
WMS_TEST_REQUIRED=1 go test ./... -v -count=1
go vet ./...
CGO_ENABLED=1 go test -race ./... -count=1
```

上面的环境变量赋值语法适用于 Bash。PowerShell 使用 `$env:WMS_TEST_REQUIRED = "1"` 等写法。

MySQL 测试通过 `internal/testutil` 创建和删除独立的 `gowms_test_*` 数据库；用 `WMS_TEST_DSN` 指定本地或专用测试实例，用 `WMS_TEST_REDIS_ADDR` 指定测试 Redis。账号需要创建测试库的权限，禁止指向生产实例。依赖不可用时部分测试默认跳过；`WMS_TEST_REQUIRED=1` 用于将缺失依赖视为失败。

Windows 本机如果没有 GCC，可以使用 `verify.ps1 -WithRace` 的 Linux 容器路径，或由 Ubuntu CI 执行 race 检测。

前端：

```bash
cd web
npm run lint
npm test
npm run build
```

端到端测试（Playwright）：

```powershell
.\scripts\e2e.ps1
```

脚本会启动独立的 Compose 项目和独立数据库，执行登录权限、入库、出库、盘点关键流程，结束后自动删除测试容器和数据卷。首次运行会构建镜像并下载 Chromium，之后可以加 `-NoBuild` 跳过镜像构建。

测试报告生成在 `web/playwright-report`。

### k6 业务流程与压测

场景、前置数据、参数与结果解释统一见 [压测指南](load-testing.md)。

## 维护约定

- 修改前阅读 [代码索引](code-index.md) 和相关测试；Go 约定见 [go-style.md](go-style.md)。
- 前端业务页面只通过 `web/src/events/businessEvents.ts` 与 Guide/Demo 交互。
- 架构展示页以 `web/public/overview.html` 为源文件；修改后在 `web/` 执行 `npm run sync:overview`，构建会检查 `docs/index.html` 是否同步。
- `web/go.mod` 是 Go 工具的扫描边界；`.cmd` 是 Windows 双击入口，实际逻辑在 `.ps1`，两者均保留。

## 本地验证记录

### 2026-10-10：并发快照与回放边界修复（评审反馈 P1×3 / P2×2）

外部评审用定向复现测试指出 3 个 P1、2 个 P2，评委复验后又指出 1 个新的 P1。两轮修复：

- 盘点审核与收货（approve.go / receiving.go）：幂等预查询**整体移出 TxRetry 回调**（独立连接、查完即还），事务内只保留业务锁读写与末尾 Insert。两轮分别消除两类问题：事务内第一条普通 SELECT 提前固定 REPEATABLE READ 读视图导致旧快照；事务占住连接后再向同一连接池申请第二条连接，池满时所有请求互相等待。交错录入实盘按最新实盘调整库存；并发收货收齐判断能看到对方提交、正常生成上架任务。
- 上架（putaway.go）：幂等回放提前到库位校验之前（库位可用性只对新操作校验），事务内不再查幂等（并发同 key 撞唯一键由 TxRetry 重跑 + reconcilePutaway 新读兜底）；完成统计的读视图在订单行锁之后固定，同单并发上架全部完成后单据正常 COMPLETED。
- 外部出库单（integration.go）：SKU 编码统一按主数据规范编码参与去重与内容比较，数据库大小写不敏感命中不再导致同内容重试误报 409。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go build ./...`、`go vet ./...`、`git diff --check` | 通过 |
| `golangci-lint run ./internal/modules/inbound/... ./internal/modules/stocktake/... ./internal/modules/outbound/... ./internal/app/...` | 0 issues（顺带清理上架测试 fixture 的 unparam 告警） |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 全量包 ok，exit 0 |
| 新增回归测试 7 个 | 实际执行：审核与实盘录入交错按最新实盘调整（库存=7 而非 8）；同单不同明细并发收货后单据 PUTAWAY 且生成 2 个上架任务（连接池上限 2）；同单两任务并发上架后单据 COMPLETED（连接池上限 2）；上架成功后禁用库位同 key 重试回放成功、新 key 才被 20011 拒绝；外部出库 SKU 大小写变体回放不 409、变体重复行 50007、快照落规范编码；连接池上限 1 下带 key 收货/审核及其重试完成 |

未运行：race、Playwright E2E、k6。本轮修改未提交。

### 2026-10-10：外部出库单同号内容校验验证（阶段 4）

在盘点审核基线上实现外部 OMS 推送出库单按业务单号的内容校验：`CreateExternal` 先解析并规范化内容（仓库按编码解析、明细按「SKU 编码+数量」集合表示且顺序无关、备注精确比较、重复编码 50007），首次查询命中与并发撞唯一键回查两条路径统一经 `replayExternalOrder` 核对——一致回放原单（`idempotent: true`），不一致 409（业务码 `50012`），历史明细缺编码快照无法可靠核对时报 `50013` 待人工处理、不放行。比较只用创建期不可变字段（`warehouse_id`/`remark`/明细 `sku_code`/`expected_qty`，全仓无更新点），不新增指纹列与迁移；业务单号去重随订单生命周期保留，不依赖 7 天请求级幂等清理。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go build ./...`、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/app/... ./internal/modules/outbound/... ./internal/pkg/errcode/...` | 0 issues |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 全量包 ok，exit 0，无 FAIL |
| 新增 `internal/app/integration_test.go` 用例 | 7 个用例实际执行：同号同内容回放（返回原单不重复建单）、明细顺序不影响相等、数量/SKU 集合/备注/仓库变化均 409（50012）、跨租户同号不同内容独立、并发同号异内容只建一张单且后到者 409、历史订单缺编码快照报 50013；原有 2 个外部集成用例回归通过 |
| 前端 | 本阶段无前端改动，未运行 npm 检查 |
| `git diff --check` | 通过（仅历史存在的 CRLF 提示） |

未运行：race、Playwright E2E、k6。本阶段与盘点审核成果均在本地未提交。

### 2026-10-10：盘点审核请求级幂等验证（阶段 3-3）

在收货/上架基线上新增 `stocktake.approve` 幂等（后端 `approve.go`，scope `stocktake.approve`，指纹=单据 ID——审核无业务请求体；空成功标记 + reconcileApprove），handler 与 demo 调用方同步加参；前端新增 `composables/stocktake/useStocktakeApprove.ts` + 接入列表/详情两个审核入口、`api/stocktake.ts`。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/stocktake/...` | 0 issues |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail |
| 后端新增 `approve_idempotency_test.go` | 4 个用例实际执行：终态后同 key 重放成功（不再误报 60002）且库存/流水/明细不重复调整、未录全拒绝不留记录且补录后同 key 可复用、同 key 用于其他单据 409、损坏标记 500、跨租户与 scope 隔离 |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；新增 `useStocktakeApprove.spec.ts` 10 个用例，全仓 148 项单测通过；构建含类型检查 |

未运行：race、Playwright E2E、k6（与阶段 2 相同的边界）。外部出库单内容校验在阶段 4 落地，验证见上方小节。

### 2026-10-10：上架请求级幂等验证（阶段 3-2）

在收货基线上新增 `inbound.putaway` 幂等：后端 `putaway.go`（scope `inbound.putaway`，指纹=任务/库位/数量，空成功标记 + reconcilePutaway），handler 与 demo 调用方同步加参；前端新增 `composables/inbound/usePutaway.ts` + 改造 `PutawayDialog.vue`、`api/inbound.ts`。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/inbound/...` | 0 issues |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail |
| 后端新增 `putaway_idempotency_test.go` | 3 个用例实际执行：同 key 重放不重复增加库存/流水/任务进度且完成后重放不重复写单据状态、同 key 改数量/库位 409、任务推进注入错误时库存/流水/库位状态/任务进度整体回滚、损坏标记 500、跨租户与 scope 隔离 |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；新增 `usePutaway.spec.ts` 11 个用例，全仓 138 项单测通过；构建含类型检查 |

未运行：race、Playwright E2E、k6（与阶段 2 相同的边界）。盘点审核幂等与外部出库单内容校验仍待实施。

### 2026-10-10：收货请求级幂等验证（阶段 3-1）

在阶段 2 基线上新增 `inbound.receive` 幂等（后端）与前端收货操作生命周期。后端修改 `receiving.go`、`handler.go`、`pkg/idempotency`（空成功标记），调用方（demo/既有测试）同步加参；前端新增 `composables/inbound/useReceive.ts` + 改造 `ReceiveDialog.vue`、`api/inbound.ts`。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/inbound/... ./internal/pkg/idempotency/...` | 0 issues（修 nilerr 2 处后） |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail |
| 后端新增 `receive_idempotency_test.go` | 4 个用例实际执行：同 key 重放不重复累计（含残品只计一次）、新 key 再收正常累计、收齐后重放不重复生成上架任务、同 key 改数量/明细/批次 409、业务失败不留记录、损坏标记 Internal、租户与 scope 隔离 |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；新增 `useReceive.spec.ts` 11 个用例，全仓 127 项单测通过；构建含类型检查 |

未运行：race、Playwright E2E、k6（与阶段 2 相同的边界）。上架、盘点审核幂等与外部出库单内容校验仍待实施。

### 2026-10-10：幂等并发回放加固验证（阶段 2）

代码基线 `main` / `c8de262`（PDA 前端操作生命周期提交之后）。本次修改 `internal/modules/outbound/service/pick.go` 并新增 `internal/app/idempotency_race_test.go`。使用本地 MySQL 8.0（`MySQL80` 服务）与 Memurai（`127.0.0.1:6379`），仅访问本地测试依赖，未连接生产实例。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件） | 通过，无输出 |
| `go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/outbound/service/... ./internal/app/...` | 通过，0 issues（修复 nilerr 4 处后） |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail，无跳过 |
| 新增并发幂等测试 | 同 key 并发部分拣货 / 拣满剩余 / 异内容 409 / 并发领取不轮换凭证 / 插入失败整笔回滚 / 损坏快照形状校验 / 租约过期回放 / 租户与 scope 隔离，全部实际执行通过 |
| 原有回归 | 取消↔拣货竞态、PDA 契约、指纹与清理等 `internal/app` 既有测试继续通过 |

未运行：race（本地无 CGO 环境，需 `verify.ps1 -WithRace` 的 Linux 容器）、Playwright E2E、k6。前端本阶段无改动，未重跑前端检查；阶段 1 的前端 lint/test/build 结果以 `c8de262` 提交为基线。

### 2026-10-10：文档更新验证

代码基线 `main` / `4e35b92`；本次只改文档，保留原有脚本改动。使用 Go 1.26.9，仅向本地 `127.0.0.1` 的测试依赖地址发起检查，没有连接生产实例。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l cmd internal migrations`、`go vet ./...` | 通过，格式检查无输出 |
| `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run` | 通过，0 issues |
| `go test ./... -count=1 -json` | 退出码 0；按含 Test 字段的终态事件统计（含子测试）：156 pass、96 skip、0 fail |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；12 个测试文件、89 项单测；构建含类型检查与 overview 同步校验 |
| 本轮修改文档的本地链接目标 / `git diff --check` | 通过；93 个本地链接目标均存在，无差异空白错误 |

后端测试显式使用本地测试 DSN、Redis 地址，`WMS_TEST_REQUIRED=0`；本地 MySQL/Redis 不可用导致 96 项跳过，包含库存并发、PDA 后端、租户隔离和迁移相关场景，**本轮不能认定这些集成场景已通过**。JSON 日志保存在执行机临时目录 `wms-maintainability-20261010-tests.jsonl`，不作为仓库持久化产物。本轮未运行 race、E2E、压测、Docker 部署或远端 CI；修改业务逻辑后应在专用依赖可用时设置 `WMS_TEST_REQUIRED=1` 复验。

### 2026-10-07：历史验证

2026-10-07，仓库文档与闲置代码整理后执行：

| 检查 | 结果 |
| --- | --- |
| gofmt、`go build ./...`、`go vet ./...` | 通过 |
| golangci-lint v2.13.2 | 0 issues |
| `go test ./... -count=1 -json` | 263 项通过，6 项 Redis 相关测试因本机依赖不可用跳过 |
| 前端 lint / Vitest / build（含类型检查） | 通过，12 个测试文件、89 项单测 |
| 文档本地链接与 `git diff --check` | 通过 |

此次没有运行 race 或 E2E，也不代表部署环境验收。后续修改需重新执行相应检查；CI 流程见 [ci.yml](../.github/workflows/ci.yml)。
