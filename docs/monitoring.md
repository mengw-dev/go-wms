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
