# GoWMS 仓储管理系统

GoWMS 是一个前后端分离的轻量级 WMS，覆盖仓库、库位、货品、入库、库存、出库、盘点、任务和系统管理。项目重点处理库存并发、FIFO 分配、单据状态机、库存流水和异步 Excel 导入。

> 适合作为 Go + Vue 全栈学习项目、毕业设计或中小型仓储系统二次开发基础。生产环境使用前请按本文的安全配置完成加固。

## 技术栈

**后端**

- Go 1.26
- Gin、GORM、MySQL 8.0.16+
- Redis 7，可选；不可用时部分能力自动降级
- JWT、bcrypt、Excelize

**前端**

- Vue 3、TypeScript、Vite 8
- Element Plus、Pinia、Vue Router、Axios
- Vitest、ESLint

## 核心能力

- 三数量库存模型：`stock = available + allocated`
- `SELECT ... FOR UPDATE` 行锁、条件更新和 CHECK 约束共同防止超卖
- 出库审核即按入库时间执行 FIFO 锁库
- 入库、出库、盘点、任务状态机
- 库存变动全量写入流水，支持来源单据和操作人追溯
- 支持分次收货、分次上架和分次拣货
- Excel 异步导入，CAS 抢占、心跳和悬挂任务补偿
- 导入任务使用“任务 ID + Excel 行号”幂等，补偿重跑不会重复建单
- JWT 登录、Token 版本失效、禁用用户即时失效、登录失败限流
- 路由级 RBAC 权限
- 异步操作日志

## 项目结构

```text
wms/
├── cmd/wms/                 # 后端入口
├── configs/                 # 本地配置
├── deploy/                  # 全栈 Docker Compose
├── docs/                    # 需求、架构、数据库、API 文档
├── internal/
│   ├── app/                 # 依赖组装与路由
│   ├── bootstrap/           # DB、Redis、迁移、种子数据
│   ├── modules/
│   │   ├── basic/           # 仓库、库位、SKU
│   │   ├── inbound/         # 入库单、收货、上架、Excel
│   │   ├── inventory/       # 库存、流水、FIFO 与锁库
│   │   ├── outbound/        # 出库单、分配、拣货、发货
│   │   ├── stocktake/       # 盘点单
│   │   ├── system/          # 用户、角色、权限、操作日志
│   │   └── task/            # 统一任务中心
│   └── pkg/                 # 配置、JWT、事务、日志、响应等公共能力
├── migrations/              # DBA 审阅用初始化 SQL
├── scripts/windows/         # Windows 一键启动/停止/重置（PowerShell + cmd 包装）
├── scripts/wms-common.ps1   # PowerShell 公共函数
├── scripts/k6/              # k6 压测与端到端脚本
├── start.cmd                # 一键启动入口（双击即可）
└── web/                     # Vue 3 前端
```

## 本地启动

### 1. 环境要求

- Go 1.26+
- Node.js 20.19+ 或 22.12+
- MySQL 8.0.16+
- Redis 7，可选

### 2. 启动后端

```bash
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS gowms DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
go run ./cmd/migrate -seed up
go run ./cmd/wms
```

服务默认监听 `http://127.0.0.1:8080`。首次启动会通过 AutoMigrate 建表并创建管理员：

```text
用户名：admin
密码：admin123
```

首次登录后请立即修改默认密码。

### 3. 启动前端

```bash
cd web
npm ci
npm run dev
```

访问 `http://127.0.0.1:5173`。开发服务器会将 `/api` 代理到 `http://127.0.0.1:8080`。

## Windows 一键启动

只需要提前安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/)。项目下载后不需要单独安装 Go、Node.js、MySQL 或 Redis。

### 方式一：双击启动

直接双击：

```text
start.cmd
```

### 方式二：PowerShell

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\windows\start.ps1
```

`start.ps1` 会自动完成：

1. 检查 Docker 和 Docker Compose
2. 检查 `.env`
3. 生成随机数据库密码
4. 生成随机 JWT 密钥
5. 构建并启动 MySQL、Redis、后端和前端
6. 等待 API 和 Web 健康检查通过
7. 自动打开浏览器

默认访问地址：

- Web：`http://127.0.0.1:80`
- API：`http://127.0.0.1:8080`
- 用户名：`admin`
- 密码：`admin123`

首次登录后请立即修改默认密码。

如果 80 或 8080 端口被占用，可以先修改 `.env`：

```env
WMS_WEB_PORT=8088
WMS_API_PORT=18080
```

然后重新运行 `.\scripts\windows\start.ps1`。

### 停止服务

```powershell
.\scripts\windows\stop.ps1
```

或者双击 `scripts\windows\stop.cmd`。停止服务不会删除数据库和上传文件。

### 清空数据并重新初始化

```powershell
.\scripts\windows\reset.ps1
```

或者双击 `scripts\windows\reset.cmd`。该操作会删除 MySQL、Redis 和上传文件 Volume，必须输入 `RESET` 确认。

### 手动 Docker 启动

```bash
cp .env.example .env
# 修改 .env 中的数据库密码和 JWT_SECRET
docker compose -f deploy/docker-compose.yaml up -d --build
```

默认情况下 MySQL 和 Redis 只在 Compose 内部网络中可用，不会暴露到宿主机公网。仅供本机 Go 开发时，可以执行：

```bash
make compose-infra
```

该命令使用 `deploy/docker-compose.dev.yaml`，只会把 MySQL 和 Redis 映射到 `127.0.0.1`。

停止服务：

```bash
make compose-down
```

## 配置

本地默认配置位于 `configs/config.yaml`。所有字段均可用环境变量覆盖：

| 环境变量 | 说明 | 默认值 |
| --- | --- | --- |
| `WMS_SERVER_PORT` | HTTP 端口 | `8080` |
| `WMS_SERVER_MODE` | `debug` / `release` | `debug` |
| `WMS_SERVER_NODE` | 雪花 ID 节点号，多实例必须唯一 | `1` |
| `WMS_SERVER_READ_TIMEOUT_SECONDS` | HTTP 读取超时 | `15` |
| `WMS_SERVER_WRITE_TIMEOUT_SECONDS` | HTTP 写入超时 | `30` |
| `WMS_SERVER_IDLE_TIMEOUT_SECONDS` | HTTP 空闲连接超时 | `60` |
| `WMS_SERVER_SHUTDOWN_TIMEOUT_SECONDS` | 优雅关停超时 | `10` |
| `WMS_SERVER_BODY_LIMIT_MB` | 请求体大小上限 | `10` |
| `WMS_SERVER_CORS_ALLOW_ORIGINS` | 允许跨域的 Origin，逗号分隔 | 仅开发默认 localhost |
| `WMS_SERVER_TRUSTED_PROXIES` | 可信反向代理 CIDR，逗号分隔 | 空 |
| `WMS_MYSQL_DSN` | MySQL DSN | 本地开发配置 |
| `WMS_REDIS_ADDR` | Redis 地址 | `127.0.0.1:6379` |
| `WMS_JWT_SECRET` | JWT 密钥，生产环境至少 32 字符 | 开发配置 |
| `WMS_INTEGRATION_API_KEY` | 外部 OMS/ERP API Key | 开发占位值 |
| `WMS_API_BIND` | API 端口绑定地址 | `127.0.0.1` |
| `WMS_API_PORT` | API 宿主机映射端口 | `8080` |
| `WMS_WEB_PORT` | Web 宿主机映射端口 | `80` |
| `WMS_UPLOAD_DIR` | Excel 上传目录 | `./data/uploads` |
| `WMS_METRICS_ENABLED` | 是否启用 Prometheus 指标端口 | `false` |
| `WMS_METRICS_PORT` | 指标服务端口 | `9090` |
| `WMS_PROMETHEUS_PORT` | Prometheus 宿主机映射端口 | `9090` |
| `WMS_METRICS_PATH` | 指标路径 | `/metrics` |
| `WMS_GRAFANA_PORT` | Grafana 宿主机映射端口 | `3000` |
| `WMS_GRAFANA_ADMIN_USER` | Grafana 管理员用户名 | `admin` |
| `WMS_GRAFANA_ADMIN_PASSWORD` | Grafana 管理员密码 | 随机生成 |

生产模式 `WMS_SERVER_MODE=release` 会拒绝过短或仍包含示例占位内容的 JWT、MySQL 和集成 API Key。多实例部署时每个实例必须使用不同的 `WMS_SERVER_NODE`。

默认情况下 API 和 Prometheus 只绑定到宿主机 `127.0.0.1`，外部访问统一通过 Web 容器的 Nginx `/api/` 代理。云服务器安全组只需要开放 `80/443` 和受限的 SSH 端口，不要开放 MySQL、Redis、API 管理端口或 Prometheus。

### Prometheus + Grafana 监控

项目内置可选的 Prometheus 和 Grafana 监控栈。Windows 一键启动监控：

```powershell
.\scripts\windows\start-monitoring.ps1
```

也可以使用：

```powershell
make compose-monitoring
```

启动后访问：

- Prometheus：`http://127.0.0.1:9090`
- Grafana：`http://127.0.0.1:3000`
- Grafana 用户名/密码：见 `.env` 中的 `WMS_GRAFANA_ADMIN_USER`、`WMS_GRAFANA_ADMIN_PASSWORD`

Grafana 会自动加载 `GoWMS Overview` 仪表盘，包含服务状态、QPS、5xx 错误率、P95 延迟、并发请求、MySQL 连接池、Goroutine 和内存。停止监控不会停止 WMS：

```powershell
.\scripts\windows\stop-monitoring.ps1
```

详细查询示例见 [docs/monitoring.md](docs/monitoring.md)。

## 权限说明

管理员角色使用 `*` 拥有全部权限，其他角色可按模块配置：

| 权限 | 用途 |
| --- | --- |
| `wms:system:user` | 用户管理 |
| `wms:system:role` | 角色管理 |
| `wms:system:log` | 操作日志 |
| `wms:basic` | 仓库、库位、货品读取与维护 |
| `wms:inventory` | 库存、汇总、流水 |
| `wms:task` | 任务中心 |
| `wms:inbound:view/create/submit/approve/cancel/receive/putaway` | 入库各阶段 |
| `wms:outbound:view/create/submit/approve/cancel/pick` | 出库各阶段 |
| `wms:stocktake:view/create/stocktake/approve/cancel` | 盘点各阶段 |

多个权限使用英文逗号分隔，例如：

```text
wms:basic,wms:inventory,wms:task,wms:inbound:view,wms:inbound:receive
```

## 测试与检查

后端：

```bash
go test ./...
WMS_TEST_REQUIRED=1 go test ./internal/... -v -count=1
go vet ./...
CGO_ENABLED=1 go test -race ./internal/... -count=1
```

Windows 本机如果没有 GCC，可以只在 Ubuntu CI 中执行 race 检测。

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

## 示例数据与第三方系统对接

可直接用于演示的 Excel、基础资料初始化脚本和模拟 OMS 推送脚本位于：

```text
samples/
```

首次初始化时会自动创建演示仓库、库位、货品和库存。详细演示步骤见 [samples/README.md](samples/README.md)。

外部系统推送出库单接口：

```text
POST /api/v1/integration/outbound-orders
X-API-Key: <WMS_INTEGRATION_API_KEY>
```

`WMS_TEST_REQUIRED=1` 会让 MySQL 不可用时直接失败，避免 CI 在集成测试全部跳过的情况下误报成功。

## Service 拆分规范

核心业务模块不再把所有方法堆在单个 `service.go` 中，而是按业务用例拆分。

```text
inbound/service/
├── service.go      # 依赖、构造函数，只保留公共装配逻辑
├── order.go        # 创建、提交、审核、取消、状态流转
├── receiving.go    # 收货
├── putaway.go      # 上架
├── import.go       # Excel 导入和补偿任务
└── query.go        # 详情、列表和查询模型
```

同样的规则应用于：

- `outbound/service`：生命周期、拣货、查询
- `stocktake/service`：创建取消、实盘录入、审核、查询
- `inventory/service`：库存变动、查询
- `system/service`：认证、用户、角色、权限、审计
- `basic/service`：仓库、库位、货品

拆分原则：

1. 同一业务用例的方法放在同一文件。
2. `service.go` 只保留依赖结构和构造函数。
3. 不为拆而拆，不创建只包一层的方法。
4. 同一模块继续使用同一个 Service 类型，避免过度拆分和循环依赖。
5. 先按用例稳定边界，再考虑未来是否拆成独立领域服务。
## ID 与接口契约

数据库内部主键仍使用 `BIGINT`，Go 代码内部仍使用 `int64`。所有对外的 `id`、`*_id` 字段在 JSON 中使用字符串传输，避免 JavaScript 超过 `Number.MAX_SAFE_INTEGER` 后发生精度丢失。

示例：

```json
{
  "id": "357813313721077761",
  "warehouse_id": "2",
  "sku_id": "30"
}
```

客户端请求也应发送字符串 ID。角色 ID 数组兼容接收字符串和数字，但服务端始终输出字符串。

## 主要 API

所有业务接口前缀为 `/api/v1`。除登录外，请求需携带：

```http
Authorization: Bearer <token>
```

主要路由：

- `POST /login`
- `GET /healthz`
- `/basic/warehouses`、`/basic/locations`、`/basic/skus`
- `/inventory`、`/inventory/summary`、`/inventory/trans`
- `/inbound/orders`、`/outbound/orders`、`/stocktake/orders`
- `/tasks`
- `/system/users`、`/system/roles`、`/system/oper-logs`

完整说明见 `docs/api.md`。

## 数据库与迁移

项目使用 `golang-migrate` 管理版本化数据库迁移，迁移文件位于 `migrations/versions`，并嵌入 `cmd/migrate` 二进制。

开发环境为了快速启动，`debug` 模式仍可执行 AutoMigrate；`release` 模式不会自动改表，必须显式执行迁移：

```bash
make migrate-up
# 或
go run ./cmd/migrate -config configs/config.yaml -seed up
```

回退一个版本：

```bash
make migrate-down
```

迁移版本记录保存在 MySQL 的 `schema_migrations` 表。生产发布应先备份数据库，再执行迁移，并检查 dirty 状态。

## 可观测性

启用 Metrics 后，服务会在独立端口提供 Prometheus 指标：

```env
WMS_METRICS_ENABLED=true
WMS_METRICS_PORT=9090
WMS_METRICS_PATH=/metrics
```

包含：

- `wms_http_requests_total`
- `wms_http_request_duration_seconds`
- `wms_http_requests_in_flight`
- `wms_db_open_connections`
- `wms_db_in_use_connections`
- `wms_db_wait_count_total`
- Go/进程运行时指标
- `wms_build_info`

启动本地 Prometheus：

```bash
make compose-monitoring
```

业务接口还提供：

```text
GET /version
GET /healthz
```

## 生产级处理

- JWT 使用版本号，用户禁用或修改密码后旧 Token 立即失效。
- 登录失败按用户名和 IP 限流。
- 操作日志对 `password`、`token`、`secret` 等字段自动脱敏。
- HTTP Server 设置读取、写入、空闲和优雅关停超时。
- 全局请求体大小受限，Excel 导入限制为 10 MB 且只接受 `.xlsx`。
- CORS 使用精确 Origin 白名单，不使用 `*`。
- 可信代理通过 CIDR 配置，避免伪造 `X-Forwarded-For`。
- 后端和前端容器均以非 root 用户运行。
- CI 执行 `go test -race`、`govulncheck`、前端依赖审计和 Docker 镜像构建。
- Prometheus 指标独立端口运行，可通过 Compose monitoring profile 启动。

## 安全建议

- 立即修改默认管理员密码。
- 生产环境必须设置独立且随机的 `WMS_JWT_SECRET`。
- 不要将生产数据库密码、Redis 密码或 `.env` 提交到 Git。
- 根据实际岗位配置最小权限，不建议普通账号使用 `*`。
- 建议在反向代理或网关层增加 HTTPS、请求体大小限制和访问日志。
- 多实例部署必须配置唯一的雪花节点号。

## License

本项目使用 [MIT License](LICENSE)。MIT 允许自由使用、复制、修改、合并、发布、分发、再许可和销售，只需保留版权和许可声明。
