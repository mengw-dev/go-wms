# 生产就绪评估

这份文档诚实列出当前项目已具备和尚未具备的生产级能力，不夸大个人项目的成熟度。适合在面试和代码审查时作为"我知道还缺什么"的参考。

## 已完成

| 能力 | 实现位置与说明 |
| --- | --- |
| CI | `.github/workflows/ci.yml`：Go 测试（含 race）、前端 lint/test/build、迁移验证 |
| E2E | `scripts/e2e.ps1` 启动独立 Compose 项目运行 Playwright，覆盖登录/权限/库存全生命周期/外部集成 |
| 健康检查 | `internal/app/server.go` 的 `/healthz` 端点，Docker Compose healthcheck |
| 结构化日志 | `internal/pkg/log`：JSON 格式，含 request_id、user_id、租户等字段 |
| 指标 | `internal/pkg/observability`：Prometheus `/metrics`，Grafana 仪表盘（monitoring profile） |
| Docker 部署 | `deploy/docker-compose.yaml`：多阶段构建、非 root 用户、持久化卷、健康检查 |
| 版本化迁移 | `cmd/migrate` + golang-migrate，迁移容器独立执行，release 不运行 AutoMigrate |
| 安全加固 | release 模式拒绝弱 JWT/默认 DSN/缺失密码；权限注册表白名单；演示重置按租户隔离 |
| 多租户隔离 | GORM 回调注入 tenant_id；平台旁路与精确租户区分；裸表查询显式过滤 |
| 并发安全 | MySQL 行锁 + FIFO 分配；k6 60 并发防超卖验证；任务执行权 run_token 校验 |
| 压测 | `scripts/k6/`：并发分配防超卖、读取/出库/拣货压力模型 |

## 尚未完成

| 能力 | 缺失原因与风险 |
| --- | --- |
| 数据库备份 | 没有 mysqldump 定时任务或快照策略；数据丢失后无法恢复 |
| PITR | 没有 binlog 点恢复流程；误操作后无法回退到精确时间点 |
| 恢复演练 | 没有从备份实际恢复并验证数据完整性的流程 |
| RPO/RTO | 未定义恢复目标和恢复时间目标 |
| 自动回滚 | 没有蓝绿发布或自动回滚机制；部署失败需手动回退 |
| 多实例 Worker | 导入消费者使用数据库队列竞争，但未验证跨实例文件存储共享和 fencing |
| 分布式限流 | 登录限流和 AI 限流均为进程内；多实例不能共享额度 |
| 统一配置中心 | 配置来自 config.yaml + 环境变量；没有动态配置更新 |
| 证书自动续期 | Caddy profile 支持 Let's Encrypt，但未验证长期运行续期 |
| 滚动升级 | Compose 重启会短暂中断；没有零停机部署策略 |
