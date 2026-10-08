# 部署与升级

[文档导航](README.md) · [项目首页](../README.md)

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
- 密码：由 `start.ps1` 自动生成并写入 `.env` 的 `WMS_ADMIN_PASSWORD`（启动结束时也会打印到终端）

`.env` 保存全部密钥，请勿提交或外传；首次登录后请按需修改密码。

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
# 设置 MYSQL_ROOT_PASSWORD、MYSQL_USER、MYSQL_PASSWORD、JWT_SECRET、WMS_INTEGRATION_API_KEY
# 设置 WMS_ADMIN_PASSWORD；开启体验功能时还需 WMS_DEMO_PASSWORD、WMS_PERSONAL_PASSWORD
docker compose --env-file .env -f deploy/docker-compose.yaml up -d --build
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

## 数据库与迁移

项目使用 `golang-migrate` 管理版本化数据库迁移，迁移文件位于 `migrations/versions`，并嵌入 `cmd/migrate` 二进制。

开发环境为了快速启动，`debug` 模式仍可执行 AutoMigrate；`release` 模式不会自动改表，必须显式执行迁移：

```bash
make migrate-up
# 或
go run ./cmd/migrate up
```

首次部署创建平台管理员（release 必须提供 `WMS_ADMIN_PASSWORD`，缺省会失败）：

```bash
make bootstrap-admin
# 或
go run ./cmd/migrate bootstrap-admin
```

### 升级已有部署（应用账户）

应用运行时使用独立账户 `wms_app`，root 只用于迁移和管理员初始化。MySQL 官方镜像
**只在首次初始化数据目录时**创建 `MYSQL_USER`，所以已有 `mysql_data` 数据卷的服务器升级时要：

1. 在服务器 `.env` 补上两项（不要依赖 Compose 中的开发默认密码）：
   `MYSQL_USER=wms_app`、`MYSQL_PASSWORD=<强口令>`；
2. 正常发布即可：`migrate` 容器先执行结构迁移，再执行 `ensure-app-user` 幂等
   创建/修复应用账户（已存在则把密码修正为当前配置，并授予所配置业务库的权限），
   `wms` 容器随后用该账户连接。

不要用 `docker compose down -v` 来升级：它会删除数据卷并重新初始化数据库。

需要手动单独执行时（连接仍使用 root）：

```bash
./migrate ensure-app-user
```

回退一个版本：

```bash
make migrate-down
```

迁移版本记录保存在 MySQL 的 `schema_migrations` 表。生产发布应先备份数据库，再执行迁移，并检查 dirty 状态。

## 生产部署注意事项

本章是部署检查项，不表示项目已经替代完整的生产运维体系。上线前仍需按业务规模完成备份恢复演练、密钥轮换、容量评估和告警值班。

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

## 自动部署与运行边界

GitHub Actions 的 CI 与 Deploy 配置位于 `.github/workflows/`。Deploy 在 main 的 CI 成功后构建并推送镜像，再通过 SSH 部署；Secrets、Variables 和 HTTPS 的配置说明见 [deploy.yml](../.github/workflows/deploy.yml)。

监控见 [monitoring.md](monitoring.md)，环境变量见 [configuration.md](configuration.md)，备份恢复、滚动升级等未完成能力见 [production-readiness.md](production-readiness.md)。
