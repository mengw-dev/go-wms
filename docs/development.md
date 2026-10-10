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
