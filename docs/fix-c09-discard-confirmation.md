# C09 / I12：业务放弃确认绑定原记录

2026-11-04，基线c7dc26e。承接演示确认子项；本项覆盖收货、上架、PDA领取/拣货、盘点审核、波次、冻结/异常及移库的本地放弃入口。C09的I24、I25仍未实施。

## 根因与实现

原确认弹窗只携带业务对象ID，用户确认后由composable重新查询最新操作key并删除。若其他页面已将旧操作清理并创建新操作，旧确认会删除新记录；部分页面还无条件显示“放弃成功”，存储写入失败或正在发送也被误报为成功。

- 收货、上架、PDA领取/拣货、盘点、波次和异常命令的discard必须接受确认前捕获的expectedKey。删除前核对当前记录仍为原key，返回是否成功；记录不存在、已替换或发送中均拒绝，不能降级为删除最新key。移库原来就传完整记录，现在增加销毁/身份归属保护。
- 三个作业弹窗复用useScopedConfirm，将显示状态、单据、选中任务和原记录纳入确认归属；关闭重开、切换目标、切换登录或离页使旧确认失效。波次列表/详情及移库同样复用作用域确认。异常通知组件向父组件传出原key，避免emit之后再决定删除对象。
- 盘点过期或冲突请求直接进入人工核对，不先弹出“确认审核”再要求放弃。仅在原记录实际删除后显示成功；存储故障保留记录，不发送业务请求。
- getOperationContext核对当前持久化登录标识，即使跨标签页storage通知尚未送达也禁止旧身份修改本地操作记录。沿用现有身份隔离、存储格式和服务端幂等，不清空历史记录。

## 修改位置

- `web/src/composables/inbound/{useReceive,usePutaway}.ts`、`outbound/{usePdaPick,useWaveCommands}.ts`、`stocktake/useStocktakeApprove.ts`、`useClosureCommands.ts`、`useStockMove.ts`、`operationLifecycle.ts`及相应测试。
- `web/src/components/{ReceiveDialog,PutawayDialog,PickDialog}.vue`、`common/ClosurePendingNotice.vue`、`outbound/WavePendingNotice.vue`，新增`OperationDialogs.confirm.spec.ts`。
- `web/src/views/outbound/{Waves,WaveDetail}.vue`、`stocktake/{StocktakeOrders,StocktakeOrderDetail}.vue`、`inventory/Moves.vue`，以及`utils/pendingOperations.ts`、代码索引。

## 验证与边界

- 定向8文件160项通过：各操作在未收到storage事件时拒绝旧key删除新key；领取、波次和异常命令覆盖同类交错；旧登录不清理记录、正常放弃不发送业务请求。
- 真实SFC setup、Pinia、业务composable与本地持久化组合28项通过：四种操作覆盖替换key、关闭重开、卸载、相同账号重新登录、路由变化、正常删除及存储失败。HTTP和弹窗交互为受控夹具，不是浏览器验收。
- 初轮定向8失败：4项失败存储夹具直接替换整个storage并丢失登录，4项多标签夹具创建另一Pinia后未切回原上下文。修正夹具仅使写入失败/恢复原Pinia，保留原业务断言；后续全部通过。日志`tmpgen/c09-fix-20261103/discard-*`保留首轮与最终结果。
- 完整前端48文件598项、lint和包含vue-tsc的生产构建均通过；git diff --check通过。
- localStorage整容器读改写仍不是跨进程原子锁，本项解决延迟确认误删，不宣称消除所有跨标签页同时写入竞争。完整存储不可写的恢复能力属于M01/I45；C08服务端在途接管不在此范围。
- 无Go/数据库迁移修改，不重复运行Go全仓。本轮浏览器未执行：此前预览启动被自动审批拒绝，仅返回“blocked by policy”，未绕过。没有推送、部署或连接生产数据库。
