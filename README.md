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
├── scripts/k6/              # k6 压测与端到端脚本
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
mysql -uroot -p < migrations/001_init.sql
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

## Docker 全栈部署

```bash
cp .env.example .env
# 修改 .env 中的数据库密码和 JWT_SECRET
docker compose -f deploy/docker-compose.yaml up -d --build
```

启动后：

- Web：`http://localhost`
- API：`http://localhost:8080`
- MySQL：`localhost:3306`
- Redis：`localhost:6379`

只启动 MySQL 和 Redis：

```bash
make compose-infra
```

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
| `WMS_MYSQL_DSN` | MySQL DSN | 本地开发配置 |
| `WMS_REDIS_ADDR` | Redis 地址 | `127.0.0.1:6379` |
| `WMS_JWT_SECRET` | JWT 密钥，生产环境至少 32 字符 | 开发配置 |
| `WMS_UPLOAD_DIR` | Excel 上传目录 | `./data/uploads` |

生产模式 `WMS_SERVER_MODE=release` 会拒绝过短或仍包含示例占位内容的 JWT 密钥。多实例部署时每个实例必须使用不同的 `WMS_SERVER_NODE`。

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
```

前端：

```bash
cd web
npm run lint
npm test
npm run build
```

`WMS_TEST_REQUIRED=1` 会让 MySQL 不可用时直接失败，避免 CI 在集成测试全部跳过的情况下误报成功。

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

应用启动时使用 GORM AutoMigrate 保证运行所需表结构，`migrations/001_init.sql` 提供可直接审阅的 MySQL 初始化脚本。生产环境建议改为版本化迁移流程，并在发布前由 DBA 审核索引和约束。

## 安全建议

- 立即修改默认管理员密码。
- 生产环境必须设置独立且随机的 `WMS_JWT_SECRET`。
- 不要将生产数据库密码、Redis 密码或 `.env` 提交到 Git。
- 根据实际岗位配置最小权限，不建议普通账号使用 `*`。
- 建议在反向代理或网关层增加 HTTPS、请求体大小限制和访问日志。
- 多实例部署必须配置唯一的雪花节点号。

## License

本项目使用 [MIT License](LICENSE)。MIT 允许自由使用、复制、修改、合并、发布、分发、再许可和销售，只需保留版权和许可声明。
