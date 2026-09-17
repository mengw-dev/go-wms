# 演示模式与双实例部署

演示模式用于把项目临时开放给 HR、面试官或朋友体验。它不是生产多租户功能，也不会改动核心 WMS 表结构；它只增加一个受控演示账号、单会话锁、数据重置和三个业务场景入口。

## 功能

- 独立演示账号：默认 `demo / demo123456`
- 单实例单会话：同一实例同一时间只允许一个有效演示会话
- 会话心跳：前端每分钟续期，默认 5 分钟无活动自动释放
- 退出即重置：正常退出、点击“退出并重置”或页面关闭时恢复初始数据
- 一键完整流程：入库、收货、上架、出库 FIFO 分配、拣货、盘点、库存调整
- 单项流程按钮：入库演示、出库演示、盘点演示
- 手动重置：演示控制台中的“重置数据”
- 双实例部署：A/B 两套 MySQL、Redis 和 Volume 完全隔离，一个被占用时可切换另一个

## 配置

环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `WMS_DEMO_ENABLED` | `false` | 是否启用演示模式；本地一键启动脚本会设置为 `true` |
| `WMS_DEMO_USERNAME` | `demo` | 演示账号 |
| `WMS_DEMO_PASSWORD` | `demo123456` | 演示密码 |
| `WMS_DEMO_SESSION_TTL_SECONDS` | `300` | 演示会话空闲超时时间 |

演示模式必须连接 Redis。Redis 中的会话键：

- `gowms:demo:active`：当前有效演示会话
- `gowms:demo:dirty`：演示数据是否需要下次进入时重置
- `gowms:demo:reset-lock`：跨实例进程的数据重置互斥锁

## 单实例本地使用

Windows：

```powershell
.\scripts\windows\start.ps1
```

启动完成后访问 `http://127.0.0.1`，使用演示账号登录。登录后右下角会出现“演示控制台”按钮。

初始化或修复演示账号：

```powershell
go run ./cmd/migrate -seed up
```

## 两个独立演示实例

准备环境文件：

```powershell
Copy-Item deploy\env.demo-a.example deploy\env.demo-a
Copy-Item deploy\env.demo-b.example deploy\env.demo-b
```

修改两份文件中的密码、JWT 密钥和 API Key。也可以直接运行下面的脚本，脚本会在首次运行时补齐默认值并生成随机密钥：

```powershell
.\scripts\windows\start-demo-a.ps1
.\scripts\windows\start-demo-b.ps1
```

两个实例的默认入口：

| 实例 | Web 地址 | API 地址 | 演示账号 |
| --- | --- | --- | --- |
| A | `http://服务器IP:18081` | `http://127.0.0.1:18080` | `demo-a` |
| B | `http://服务器IP:18082` | `http://127.0.0.1:18090` | `demo-b` |

脚本使用不同的 Compose 项目名和 Volume：

```text
gowms-demo-a: deploy/env.demo-a
gowms-demo-b: deploy/env.demo-b
```

因此 A/B 的 MySQL 数据、Redis 会话和上传文件互不影响。API 默认只绑定 `127.0.0.1`，外部只需要通过 Web 反向代理访问。

停止实例：

```powershell
.\scripts\windows\stop-demo-a.ps1
.\scripts\windows\stop-demo-b.ps1
```

如果使用 Linux 服务器，可将 `deploy/env.demo-a` 作为 `--env-file`：

```bash
docker compose -p gowms-demo-a --env-file deploy/env.demo-a -f deploy/docker-compose.yaml up -d --build
docker compose -p gowms-demo-b --env-file deploy/env.demo-b -f deploy/docker-compose.yaml up -d --build
```

## HR/面试官使用说明

可以把下面这段直接发给对方：

> 演示地址：http://你的公网IP:18081  
> 用户名：demo-a  
> 密码：见你收到的演示凭据  
>
> 登录后点击右下角“演示控制台”。点击“一键完整流程演示”会自动跑一遍入库、出库和盘点。需要恢复初始数据时点击“重置数据”或“退出并重置”。同一实例同一时间只允许一个演示会话，请勿多人同时使用；如果提示环境被占用，请等待约 5 分钟或使用备用实例 B。

## 压测和 Grafana 展示

正常业务流程、k6 并发压测和 Grafana 指标查看见 [load-testing.md](load-testing.md)。推荐把 A 实例用于压测，B 实例保留给 HR 正常操作，避免压测数据和演示数据互相影响。

## 安全边界

演示账号拥有 `wms:demo` 以及演示所需业务权限，但没有系统用户、角色和操作日志管理权限。生产环境仍必须：

- 使用 HTTPS
- 修改默认管理员密码
- 把密码和 JWT 密钥放到环境变量或密钥管理系统中
- 只开放 `80/443` 和受限的 SSH 端口
- 不要开放 MySQL、Redis、API 管理端口和 Prometheus
- 不要把真实业务数据放进演示实例