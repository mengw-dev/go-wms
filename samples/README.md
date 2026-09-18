# WMS 演示数据与上下游模拟

应用首次初始化时会自动写入一套精简演示基础资料。本目录提供可手动导入的入库 Excel、基础资料修复脚本和模拟 OMS 推送工具，便于演示完整业务链路。

## 1. 准备基础资料

如果当前数据库没有 `WH01` 和 `SKU000001` 至 `SKU000005`，可以执行下面的幂等修复脚本：

```powershell
.\samples\setup-demo.ps1
```

脚本会通过现有管理 API 幂等创建：

- 仓库：`WH01`
- 库位：`A01-01-01` 等格式的演示库位
- 货品：`SKU000001` 至 `SKU000005`

如果 WMS 不在 `http://127.0.0.1`，可以指定地址：

```powershell
.\samples\setup-demo.ps1 -BaseUrl http://127.0.0.1:8081
```

## 2. 入库 Excel 导入演示

文件位置：

```text
samples/inbound/入库单批量导入示例.xlsx
samples/inbound/入库单批量导入示例_含错误行.xlsx
```

登录 WMS 后：

1. 进入"入库管理 / 入库单"。
2. 点击"Excel 导入"。
3. 选择上面的 Excel 文件。
4. 点击"开始导入"，观察成功、失败行数以及错误原因。

正常样本含 **100 行**数据，适合演示异步导入的全过程：PENDING → PROCESSING → COMPLETED，可在导入页面轮询看到实时进度。

第二个文件含 30 行（22 行正常 + 8 行错误），故意包含错误仓库、错误货品、非法数量等，适合演示部分成功和错误明细。

## 3. 模拟 OMS 推送出库单

外部系统接口：

```text
POST /api/v1/integration/outbound-orders
X-API-Key: <WMS_INTEGRATION_API_KEY>
```

`start.ps1` 会在 `.env` 中生成 `WMS_INTEGRATION_API_KEY`。运行示例 OMS 脚本：

```powershell
.\samples\outbound\push-outbound.ps1
```

如果 WMS 使用其他地址：

```powershell
.\samples\outbound\push-outbound.ps1 -BaseUrl http://127.0.0.1:8081
```

示例文件 `outbound-orders.json` 包含 3 张 OMS 出库单。脚本重复执行不会重复建单，会返回 `idempotent: true`，用于演示业务单号幂等。

推送后的出库单状态为 `DRAFT`，可在"出库管理"中继续提交、审核和拣货。若要完成拣货，需要先通过入库 Excel 或手工入库形成可用库存。
