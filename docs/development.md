# 本地开发与验证

[文档导航](README.md) · [项目首页](../README.md)

以下命令均从仓库根目录执行，另有说明的除外。

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
.\scripts\windows\verify.ps1
```

需要同时运行 Linux race 检查时：

```powershell
.\scripts\windows\verify.ps1 -WithRace
```

`verify.ps1` 和 `e2e.ps1` 会先检查 Node.js 版本；要求为 20.19+ 或 22.12+。

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
