# 业务流程模拟与 k6 压测

这个页面用于把 WMS 的“正常操作”和“并发压测”讲清楚，适合演示给体验者、面试官或技术评审。

## 三层验证

| 层级 | 目标 | 入口 |
| --- | --- | --- |
| 1. 正常业务流程 | 证明入库、上架、出库、盘点状态机完整可跑通 | 页面“业务流程中心 → 一键完整流程”，或 `k6 demo-flow.js` |
| 2. 出库冒烟 | 验证一张出库单从 DRAFT 到 SHIPPED | `k6 outbound-e2e.js` |
| 3. 真实并发 | 观察库存锁、FIFO 分配、库存不足拒绝和任务防超拣结果 | `k6 outbound-stress.js` / `pick-stress.js` |

## 前置条件

启动服务：

```powershell
.\scripts\windows\start.ps1
```

压测建议使用独立环境（如 `scripts/e2e.ps1` 启动的 e2e 环境）或专用的管理员账号，避免压测数据影响演示账号（demo1~demoN）的体验数据。

确认 k6 可用：

```powershell
k6 version
```

如果本机没有 k6，可以从 [k6 官方安装文档](https://grafana.com/docs/k6/latest/set-up/install-k6/) 安装。

## 第一步：环境自检

```powershell
.\scripts\windows\run-k6.ps1 -Mode check -BaseUrl http://127.0.0.1:8080
```

脚本会自动找到：

- `WAREHOUSE_ID`
- `SKU_ID`
- 当前可用库存

如果输出中没有 ID，先执行演示实例的“一键完整流程演示”或 `cmd/migrate -seed up` 初始化基础数据。

## 第二步：跑正常业务流程

```powershell
.\scripts\windows\run-k6.ps1 -Mode flow -BaseUrl http://127.0.0.1:8080
```

脚本执行：

1. 创建入库单
2. 提交并审核
3. 收货
4. 上架增加库存
5. 创建出库单
6. 审核时 FIFO 分配
7. 拣货并发货
8. 创建盘点单
9. 录入实盘数量
10. 审核盘点并调整库存
11. 校验库存不为负数，且 `stock = available + allocated`

这套流程和页面“一键完整流程”使用同一套核心业务服务，区别只是入口不同。

业务流程中心还提供“并发出库分配实验”，体验者可以选择并发张数和每张数量，页面会显示实验重点、总需求量、成功/拒绝数量和最终库存；“运行状态与指标快照”页面会显示连接池、协程、内存和库存数量。

## 第三步：出库冒烟

```powershell
.\scripts\windows\run-k6.ps1 -Mode smoke -BaseUrl http://127.0.0.1:8080 -WarehouseId 21 -SkuId 69 -OrderQty 1
```

替换成第一步输出的真实 `WAREHOUSE_ID` 和 `SKU_ID`。

## 第四步：并发压测

### 出库混流压测

20 VU 并发创建、提交、审核和拣货，主要验证：

- 库存行锁排队
- 乐观锁 version 冲突
- `TxRetry` 死锁重试
- 库存不足时整体回滚，不产生负库存

```powershell
.\scripts\windows\run-k6.ps1 -Mode stress -BaseUrl http://127.0.0.1:8080 -WarehouseId 21 -SkuId 69
```

快速演练可以先缩小规模：

```powershell
$env:STRESS_VUS = "5"
$env:STRESS_RAMP = "2s"
$env:STRESS_PEAK = "5s"
$env:STRESS_DOWN = "2s"
k6 run -e BASE_URL=http://127.0.0.1:8080 -e WAREHOUSE_ID=21 -e SKU_ID=69 scripts/k6/outbound-stress.js
```

### 真实波次拣货压测

40 个库位铺货，40 个拣货员逐件扫码，20 个抢单 VU 模拟重复扫码。默认数据规模较大，适合在专用压测实例运行：

```powershell
.\scripts\windows\run-k6.ps1 -Mode wave -BaseUrl http://127.0.0.1:8080 -WarehouseId 21 -SkuId 69
```

该脚本最终重点核对：

- 出库单最终为 `SHIPPED`
- `picked_qty` 恰好等于订单量
- 所有任务 `COMPLETED`
- 没有负库存
- 并发重复拣货被业务拒绝，不会超拣

## 接入 Prometheus + Grafana

启动监控：

```powershell
.\scripts\windows\start-monitoring.ps1
```

在另一个终端运行时把 k6 指标写入 Prometheus：

```powershell
.\scripts\windows\run-k6.ps1 -Mode stress -BaseUrl http://127.0.0.1:8080 -WarehouseId 21 -SkuId 69 -RemoteWrite
```

同时打开 Grafana：

```text
http://127.0.0.1:3000
```

体验者可以重点看：

- WMS 请求速率是否随 k6 流量上升
- P95 响应时间是否出现上升
- 5xx 错误率是否保持接近 0
- MySQL 连接池是否被压满
- Goroutine 和内存是否稳定

k6 的远端指标也可以通过 Prometheus Explore 查看，例如：

```promql
k6_http_reqs_total
```

## 给体验者的演示话术

可以按下面顺序演示：

1. 先打开“演示控制台”，点击“一键完整流程演示”。
2. 说明系统刚完成入库、上架、FIFO 分配、拣货和盘点。
3. 打开“任务中心”，展示任务状态从待执行到完成。
4. 打开“库存查询”和“库存流水”，说明库存不能为负数。
5. 运行 `run-k6.ps1 -Mode stress -RemoteWrite`，展示 Grafana 的 QPS、P95 和错误率。
6. 压测结束后点击“重置数据”，恢复初始演示状态。

## 注意

- 压测不要直接打到真实生产数据库。
- 默认演示库存不足以支撑无限并发出库，业务失败通常是可用库存不足触发的业务拒绝，不一定是系统故障。
- 压测脚本中的业务失败计数需要和最终库存、订单状态一起看，不能只看 HTTP 200。