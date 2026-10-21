# 业务流程模拟与 k6 压测

这个页面用于把 WMS 的“正常操作”和“并发压测”讲清楚，适合演示给体验者、面试官或技术评审。

本地有界规模、仓库行锁和正式迁移查询计划的2026-10-21验证见[规模验证（D25）](scale-validation.md)。默认200明细/1000分配行限制也影响压测输入，超限拒绝不应误报为库存不足。

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

如果输出中没有 ID，先执行演示实例的“一键完整流程演示”或 `cmd/migrate seed-demo` 初始化基础数据。

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

## 并发分配证明后的清理（D22）

`allocation-proof.js` 故意留下分配占用，清理入口为 `scripts/k6/cleanup-allocation.js`。它仅通过已有业务取消接口释放分配，保留单据、任务和流水；不执行 SQL、不删除单据、不补货。只在专用本地/测试环境使用，运行前停止目标范围的压测、建单、编辑和拣货。使用明确指定的持久普通测试账号；平台租户 0 会被拒绝，原有 API 仍执行服务端签名、权限及租户校验。

先只读预览（默认不取消）：

```powershell
k6 run -e BASE_URL=http://127.0.0.1:8080 -e K6_USERNAME=user1 -e WAREHOUSE_ID=21 -e SKU_ID=69 scripts/k6/cleanup-allocation.js
```

需要执行取消时显式设置开关；账号密码沿用公共脚本的 `K6_PASSWORD` 配置：

```powershell
k6 run -e BASE_URL=http://127.0.0.1:8080 -e K6_USERNAME=user1 -e WAREHOUSE_ID=21 -e SKU_ID=69 -e CLEANUP_EXECUTE=1 scripts/k6/cleanup-allocation.js
```

- 仓库和 SKU 必填，按正 int64 字符串处理，不能使用模糊 ID 或 JavaScript 数值转换。可用 `CLEANUP_PREFIX` 缩小已存在的测试单号前缀，必须以 `K6PROOF` 开头，默认 `K6PROOF`。前缀是测试数据约定，不是服务端不可变的来源证明。
- 订单/库存均按接口 total 完整分页，检查缺页、重复、总数变化和响应形状；每类最多 100000 行，超出预算明确失败而非静默截断。服务端关键词是模糊查询，客户端再次核对精确前缀、租户、仓库，并读取每张候选单的全部明细；其他 SKU 或混合 SKU 整单排除，不能按其中一行命中就取消。
- 已取消单可重复预览且不再取消；已发货、已有拣货/任务进度、主单与分配明细不一致时停止。目标 SKU 总分配必须恰好等于候选单的活动分配之和，否则表明其他占用或数据不完整，停止且不扩大取消范围。
- 执行前按固定顺序的业务字段再读核对，每批最多 200 单。检查批次成功/失败计数和错误明细，逐单确认 CANCELLED，再完整分页复核现存量不变、分配清零、可用量按取消量回补。候选预览最多打印前 20 个 ID，完整数量和计划释放量另行打印。
- 失败通过 checks 阈值及 `execution.test.abort` 返回非零退出码。网络/非 JSON/批次部分成功/取消响应未知均停止后续操作，不自动重试、不回滚此前成功批次；先查询业务结果，再重新预览。日志不输出 token 或整段登录响应。
- 当前取消 API 只接受 ID，客户端两次核对与服务端取消之间仍有时间间隙，不能宣称能在并发编辑中原子锁住范围。停止其他写入是运行前提；服务端原有取消事务与锁内未拣货校验保持不变。

无 WMS 或数据库也可运行脚本回归：

```powershell
node --experimental-vm-modules --test scripts/k6/cleanup-allocation.test.mjs
node --test scripts/k6/cleanup-allocation.k6.test.mjs
```

第二条需要 PATH 中有 k6，或设置 `K6_BIN` 指向本机可执行文件；它启动回环 HTTP 夹具并运行真实 k6，不连接实际 WMS。覆盖大 ID、超过 10 页、库存超过 100 条、混合范围排除、分批部分失败及实际进程退出码。VM 模块开关仅用于 Node 测试，不是 k6 运行要求。

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