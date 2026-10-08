# 配置参考

[文档导航](README.md) · [项目首页](../README.md)

本地默认配置位于 `configs/config.yaml`。以下列出常用环境变量；配置字段与校验以 `internal/pkg/config` 为准，Compose 插值项以 `deploy/docker-compose.yaml` 为准。Docker 示例见 [`.env.example`](../.env.example)。

表内默认值主要指本机应用配置，Compose 会覆盖部分值（例如运行模式和指标开关）。

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
| `WMS_INTEGRATION_TENANT_ID` | 外部 API Key 绑定租户，0 表示精确平台租户 | `0` |
| `WMS_INTEGRATION_API_KEY` | 外部 OMS/ERP API Key | 开发占位值 |
| `WMS_ADMIN_PASSWORD` | 平台管理员初始密码（release 首次部署必须设置，缺失会导致初始化失败） | 开发默认 `admin123` |
| `WMS_DEMO_ENABLED` | 演示模式总开关（账号 + `/demo` 接口，`false` 时路由完全不挂载） | `true` |
| `WMS_DEMO_INSTANCES` | 演示账号数量（demo1~demoN，各自独立租户） | `5` |
| `WMS_DEMO_PASSWORD` | 演示账号密码（所有演示账号共用；release 开启演示时必须显式设置） | 开发默认 `demo123456` |
| `WMS_DEMO_SESSION_TTL_SECONDS` | 演示会话空闲超时秒数 | `300` |
| `WMS_API_BIND` | API 端口绑定地址 | `127.0.0.1` |
| `WMS_API_PORT` | API 宿主机映射端口 | `8080` |
| `WMS_WEB_PORT` | Web 宿主机映射端口 | `80` |
| `WMS_UPLOAD_DIR` | Excel 上传目录 | `./data/uploads` |
| `WMS_UPLOAD_FAILED_FILE_RETENTION_HOURS` | 失败导入文件保留小时数（到期后台清理，成功文件立即删除） | `72` |
| `WMS_METRICS_ENABLED` | 是否启用 Prometheus 指标端口 | `false` |
| `WMS_METRICS_PORT` | 指标服务端口 | `9090` |
| `WMS_PROMETHEUS_PORT` | Prometheus 宿主机映射端口 | `9090` |
| `WMS_METRICS_PATH` | 指标路径 | `/metrics` |
| `WMS_GRAFANA_PORT` | Grafana 宿主机映射端口 | `3000` |
| `WMS_GRAFANA_ADMIN_USER` | Grafana 管理员用户名 | `admin` |
| `WMS_GRAFANA_ADMIN_PASSWORD` | Grafana 管理员密码 | 随机生成 |

生产模式 `WMS_SERVER_MODE=release` 会拒绝过短或仍包含示例占位内容的 JWT、MySQL 和集成 API Key；开启演示/体验账号时还要求显式提供 `WMS_DEMO_PASSWORD` / `WMS_PERSONAL_PASSWORD`。首次部署必须提供 `WMS_ADMIN_PASSWORD` 才能创建平台管理员（管理员已存在后重启不再要求）。多实例部署时每个实例必须使用不同的 `WMS_SERVER_NODE`。

默认情况下 API 和 Prometheus 只绑定到宿主机 `127.0.0.1`，外部访问统一通过 Web 容器的 Nginx `/api/` 代理。云服务器安全组只需要开放 `80/443` 和受限的 SSH 端口，不要开放 MySQL、Redis、API 管理端口或 Prometheus。


## Compose 应用账户

手动部署需设置 `MYSQL_ROOT_PASSWORD`（迁移账户密码）、`MYSQL_USER`（应用账户，通常为 `wms_app`）、`MYSQL_PASSWORD`（应用账户密码）和 `MYSQL_DATABASE`（默认 `gowms`）。Windows 启动脚本会生成所需的随机密码；手动部署不要沿用 Compose 的开发默认密码。

## 持久体验账号

`WMS_PERSONAL_ENABLED`、`WMS_PERSONAL_INSTANCES` 和 `WMS_PERSONAL_PASSWORD` 控制持久体验账号。release 模式开启时必须显式提供密码；真实业务部署应同时关闭演示与持久体验账号。

监控启动和指标说明见 [monitoring.md](monitoring.md)，完整部署步骤见 [deployment.md](deployment.md)。
