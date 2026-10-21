# WMS 监控说明

监控链路：

```text
WMS /metrics
  -> Prometheus 每 15 秒采集
  -> Grafana 展示 WMS Overview
```

## 启动

```powershell
.\scripts\windows\start-monitoring.ps1
```

访问地址：

```text
Prometheus: http://127.0.0.1:9090
Grafana:    http://127.0.0.1:3000
```

Grafana 管理员密码保存在 `.env` 的 `WMS_GRAFANA_ADMIN_PASSWORD`。

## 面板内容

| 面板 | 含义 |
| --- | --- |
| 服务状态 | Prometheus 是否能采集 `gowms` |
| 请求速率 | 每秒 HTTP 请求数 |
| 5xx 错误率 | 服务端错误占比 |
| P95 响应时间 | 95% 请求的耗时上限 |
| 当前并发请求 | 正在处理中的请求数 |
| 接口请求速率 | 按路由拆分 QPS |
| 状态码请求速率 | 按 HTTP 状态码拆分 QPS |
| MySQL 连接池 | Open / In Use / Idle |
| Goroutine 数量 | Go 协程数量 |
| 进程内存 | 服务 RSS 内存 |
| 业务事务 P95 / 重试速率 | 事务每次尝试的耗时与实际发起的额外尝试，区分死锁和业务冲突 |
| 导入执行 P95 / 恢复次数 | 已领取任务的执行耗时、过期执行权归还次数 |
| 导入积压 / 最早待导入时长 | 全租户当前待处理、处理中数量，以及最早待处理任务距创建的时间 |
| MySQL 行锁等待 | 整个 MySQL 实例的当前等待数与累计等待耗时速率 |
| 数据库观测采集状态 | 导入队列、MySQL 行锁采集是否成功；失败不输出虚假的零值 |

## 业务与导入指标（D26）

沿用现有 `/metrics` 与 Grafana，不增加组件或依赖。仅在启用 Metrics 并完成应用装配后采集；业务指标不带单号、文件路径、用户或租户标签，避免泄露与无限增长的时间序列。

| 指标 | 口径 |
| --- | --- |
| `wms_transaction_attempt_duration_seconds` | 经 `tx.Manager` 的每次事务尝试，包含数据库调用与锁等待；`outcome` 为 success/error/conflict/deadlock/lock_timeout/canceled/panic。不是完整业务请求耗时，也不覆盖绕开事务管理器的 SQL。完整 HTTP 耗时仍看原有路由直方图。 |
| `wms_transaction_retries_total` | 真正开始的额外尝试；reason 为 deadlock/conflict。第一次执行、退避中取消、窗口耗尽均不增加重试数；1205 不自动重试。 |
| `wms_import_run_duration_seconds` | 每次成功领取到保存结果/归还执行权的耗时；outcome 为 completed/partial/failed/released/lost/error。completed 和 partial 均为任务 COMPLETED，partial 表示有行失败；released 表示取消后归还，lost 表示已失去执行权，error 表示本次无法完成状态持久化。 |
| `wms_import_recoveries_total` | 补偿器成功把过期 PROCESSING 归还 PENDING 的次数，不是最终成功任务数。 |
| `wms_import_queue_tasks{status}` | 每次抓取从数据库聚合全租户 PENDING/PROCESSING，排除软删除。多应用副本共享数据库时用 max 聚合，不能相加。 |
| `wms_import_oldest_pending_age_seconds` | 当前 PENDING 中最早创建任务的年龄（恢复任务也从原创建时间计算）；空队列为0。 |
| `wms_mysql_row_lock_current_waits` | MySQL 实例级当前 InnoDB 行锁等待，不是应用连接池等待。 |
| `wms_mysql_row_lock_waits_total` / `wms_mysql_row_lock_wait_seconds_total` | MySQL 实例累计等待次数/秒数；同一数据库被多个应用副本采集不能直接相加，也不能归因为某个租户/仓库。 |
| `wms_import_queue_scrape_success` / `wms_mysql_row_lock_scrape_success` | 1 成功、0 失败。每类查询最多等待2秒，失败时省略对应数据指标，其余指标仍可采集。权限不足或 MySQL 不提供所需状态变量也按失败处理。 |

行锁数据取自 `SHOW GLOBAL STATUS` 的 `Innodb_row_lock_*`；MySQL 原始耗时单位为毫秒，输出时换算为秒。连接池的 `wms_db_wait_*` 含义保持不变。参见 [MySQL 状态变量](https://dev.mysql.com/doc/refman/8.0/en/server-status-variables.html)、[Prometheus exporter 指南](https://prometheus.io/docs/instrumenting/writing_exporters/)。

查询失败时先检查采集状态，不把面板的无数据当作“没有积压/没有行锁”。累计值随进程或数据库重启归零，查看变化率使用 `rate`/`increase`。直方图的分位数只适合有足够样本时评估，当前仪表不代替 D25 的受控压测和业务限额决策。

## 导入异常排查与恢复

上传只落文件和 PENDING 任务，由常驻 worker 领取。正常取消会归还自己的执行权；强制终止进程时，PROCESSING 在心跳超过5分钟后由每2分钟扫描的补偿器归还，再逐行重跑，已有行按任务+行号复用。不是一启动就抢走所有 PROCESSING；持续心跳的执行不能被重置。

源文件丢失会进入 FAILED，提示“核对已生成单据后重新上传”，不会反复进入待处理。若已部分建单后丢文件，已提交的单据仍存在；此时任务计数反映本次未读到文件的执行，不能据零计数判断历史未建单。先按导入批次查询已生成单据，再决定如何补齐，避免整份再次上传产生新的任务。恢复必须保留原文件与同一任务；本地文件不支持任意多节点部署，需要同路径共享持久卷，边界 B03 保留。

重置/重传前可先查看积压年龄、处理中数量、恢复计数及结构化日志中的 task_id；观测指标本身不改变业务任务。D26 回归会真实启动并强制终止导入子进程，确认第一行已提交、第二行未提交，再仅将持久化心跳时间回退以验证过期恢复，省去真实等待5分钟。

## 常用 PromQL

```promql
sum(rate(wms_http_requests_total[1m]))
```

```promql
100 * (sum(rate(wms_http_requests_total{status=~"5.."}[5m])) or vector(0))
  / clamp_min(sum(rate(wms_http_requests_total[5m])), 0.001)
```

```promql
histogram_quantile(
  0.95,
  sum by (le) (rate(wms_http_request_duration_seconds_bucket[5m]))
)
```

```promql
wms_db_in_use_connections
```

```promql
go_goroutines
```

## 远程服务器

Prometheus 和 Grafana 默认只绑定服务器本机。通过 SSH 隧道查看：

```powershell
ssh -L 3000:127.0.0.1:3000 -L 9090:127.0.0.1:9090 root@你的服务器IP
```

然后本机访问：

```text
http://127.0.0.1:3000
http://127.0.0.1:9090
```

不要把 Grafana 或 Prometheus 无鉴权直接暴露到公网。需要公网演示时，应通过带 HTTPS 和登录保护的反向代理发布 Grafana。
