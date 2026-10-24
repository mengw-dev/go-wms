# 操作恢复专项验收（2026-10-24）

基线：`cd37d60`。本轮继续检查异常结果恢复和权限组合，不重复已通过的完整收发货、冻结/退货事务、移库、普通表格布局或整个历史测试集。

## 发现与修复

| 编号 | 级别 | 已确认行为 | 修复 |
|---|---|---|---|
| F33 | P1 | HTTP成功但返回空对象、数字/错误对象ID，或不完整的PDA快照时，前端提前删除待确认记录，可能失去原请求恢复依据。 | 冻结、异常处理、退货、波次按现有接口核对回执ID；已有对象必须与原请求一致。PDA核对状态、非负整数数量、领取凭证及时间。不完整响应保留原请求并提示恢复，不覆盖原快照。 |
| F34 | P2 | 冻结、退货/异常、波次作业出错后，恢复按钮位于当前弹窗后方，用户无法直接点击。 | 恢复提示放到当前弹窗内，关闭后保留页面入口；覆盖创建波次、汇总拣货/播种分货、异常结束、冻结及异常/退货共用表单。闭环记录放弃确认使用已有页面/账号保护。 |
| F35 | P2 | 波次只读账号仍查询基础资料；仓库选项临时失败后没有重试按钮，只能重新进入页面。 | 基础资料查询按权限执行；选项失败有明确重试入口，加载/失败时禁止创建，波次列表独立显示。 |

回执ID始终保留为字符串，校验正数及有符号BIGINT上界，不经JavaScript数字转换。新异常、退货和波次只能核验新ID是否合法；已有冻结库存、异常处理和波次动作同时核对原对象ID。此检查不能证明响应来自可信服务，也不代替后端事务和幂等记录。

PDA快照允许合法的零剩余数量；领取幂等回放可以返回原先已过期的租约，回执检查不误拒该结果，是否允许后续作业继续由既有租约界面及服务端裁决。收货、上架等原有合法空成功响应没有套用非空回执要求。

## 验证

- 修复前19项新增回执故障断言均失败；修复后20项回执单测及125项直接相关既有单测通过（6文件，合计145项）。包括原key恢复、错误对象/大ID、非法数量/凭证、不覆盖原快照，以及过期领取回放和合法零剩余。
- 最终生产构建预览12项浏览器测试通过，0失败、0重试。冻结、客户退货、异常结束波次、汇总拣货和新建波次，各验证成功响应缺字段与网络断开两种故障；另验证只读波次不读取基础资料，以及仓库选项503后重试。
- 上述10项恢复场景均使用390px夜间视口，直接点击当前弹窗内恢复按钮；确认两次写请求使用相同key及业务参数、明确回执后才清除本机记录，且页面没有横向溢出。
- 浏览器用本地受控API模拟已接收请求后的响应异常；测试中的接收集合只用于核对相同key，不把模拟集合当作真实MySQL去重证明。本轮没有连接生产数据库。
- ESLint、Vue/TypeScript检查、前端构建及差异空白检查通过。后端源码、迁移和数据库规则没有变更，本轮不重跑MySQL集成测试，沿用既有事务验收记录。

自动化入口：`web/src/composables/commandReceipts.spec.ts`、`web/tests/e2e/recovery-audit.spec.ts`。本地原始证据保存在忽略目录`tmpgen/recovery-audit-20261024/`，含修复前浏览器产物与`production-results.json`。可复跑专项：

```powershell
Set-Location web
node node_modules/vitest/vitest.mjs run src/composables/commandReceipts.spec.ts src/composables/useClosureCommands.spec.ts src/composables/outbound/useWaveCommands.spec.ts src/composables/outbound/usePdaPick.spec.ts src/composables/operationRecovery.spec.ts src/composables/operationLifecycle.spec.ts --maxWorkers=2
# 先构建并启动本地preview，再运行浏览器专项；接口由用例拦截，不需启动业务后端。
$env:E2E_BASE_URL='http://127.0.0.1:4187'
node node_modules/@playwright/test/cli.js test tests/e2e/recovery-audit.spec.ts
```

D27持续审查及B类运行边界保持开放；本轮通过不代表全仓没有隐藏问题，Git推送和目标环境部署状态另行确认。
