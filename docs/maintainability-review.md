# 可维护性与可扩展性评审（2026-10-10）

[文档导航](README.md) · [迭代路线图](iterations.md) · [工程约定](go-style.md)

## 1. 结论与范围

当前代码具备继续演进的基础：模块化单体、显式依赖组装、具体 Repository、跨模块事务接口，以及库存不变量和竞态回归测试都值得保留。可维护性较好，但规范的自动执行和文档一致性仍需加强；在现有单库内增加业务用例的扩展成本可控，多实例与大单作业则有明确边界。

本次基于 `main`、HEAD `4e35b92` 及当前工作区评审。阅读了启动组装、出库拣货/领取、库存分配、上架、系统用户角色事务、租户与幂等基础设施、PDA 前端和相关测试，并检查 lint/CI 配置及主要工程文档。这是重点调用链评审，不是全仓逐行验收、性能复测或生产就绪认证。本次只更新文档。

| 维度 | 判断 | 依据与限制 |
| --- | --- | --- |
| Go 风格与可读性 | 较好 | 构造函数显式、错误大多按类型分类、业务按用例拆文件；局部仍有按错误文本分类和过宽的 lint 豁免。 |
| 职责与依赖 | 较好，缺自动守卫 | `app.New` 集中装配，出库通过 basic/inventory/task 的 API 协作；现有 lint 尚未配置 depguard，前端也未配置依赖方向限制。 |
| 业务测试 | 有价值，但分布不均 | 有真实锁竞争、回滚和数量不变量断言；PDA 目前的六项单测未覆盖连续成功、切换任务、刷新恢复。 |
| 单库业务扩展 | 可控 | 库存与任务 API 共享调用方事务；新增流程需要保留锁顺序、租户、状态机、流水和幂等约束。 |
| 多实例与规模扩展 | 有条件 | 导入依赖本地文件，部分缓存/限流在进程内；详情任务固定取前 200 条，不能把分页常量当业务上限。 |
| 文档维护 | 本轮已纠偏，仍需持续核对 | 同一规则在规范、架构、迭代、历史记录中重复，曾出现互相冲突的事务规则与过期测试待办。 |

## 2. 应保留的设计

- **具体类型加小接口**：[app.New](../internal/app/app.go) 让依赖可追踪；[Basic Service](../internal/modules/basic/service/service.go) 的 `redisClient` 由消费者定义。无需为了可测试性把每个 Repository 都改成接口。
- **事务由用例持有**：[InventoryAPI](../internal/modules/inventory/api/api.go) 明确接收调用方 `*gorm.DB`；[Pick](../internal/modules/outbound/service/pick.go) 将任务、分配、库存和幂等结果放在同一事务。接口携带 GORM 是当前单库的明确取舍，不等于可直接替换成 RPC。
- **测试验证业务结果**：[库存并发测试](../internal/modules/inventory/service/service_test.go) 不只检查成功数量，还检查失败类型与库存等式；[拣货竞态测试](../internal/app/pick_race_test.go) 控制取消/拣货交错并检查终态；[盘点测试](../internal/modules/stocktake/service/service_test.go) 已覆盖锁等待后按当前账面计算差异。
- **前端业务事件边界**：[出库页面](../web/src/views/outbound/OutboundOrders.vue) 经 `businessEvents` 发出事件；保留业务页面与 Guide/Demo 的单向关系，不为演示需求向业务层加入反向依赖。

## 3. 已确认的改进项

优先级表示后续处理顺序。下列业务问题仅登记，未在本轮修改实现。

### P1：PDA 操作状态没有跟随一次业务操作结束

证据：[usePdaPick](../web/src/composables/outbound/usePdaPick.ts)、[PickDialog](../web/src/components/PickDialog.vue)、[现有六项测试](../web/src/composables/outbound/usePdaPick.spec.ts)。

- 部分拣货成功后弹窗保留，`pickKey` 仍沿用首次 UUID。再次真实拣同样数量会回放旧结果而不累加；修改数量会触发同 key 不同内容的 409。
- 切换下拉框任务只更新任务 ID、数量和扫描字段，`claimToken`、`claimKey`、`pickKey` 与快照未切换；页面仍显示已领取，提交新任务可能被凭证或幂等校验拒绝。
- 超时后关闭重开或刷新会丢失原 key；如果首次请求已提交但响应丢失，再生成新 key 可能重复累计。
- `claim()` 的 key 也一直复用，不能把“回放首次领取响应”当成新一次续领；租约过期后的重新领取需要独立操作状态。

验收应覆盖：两次合法同参部分拣货各执行一次、超时重试不重复执行、切换任务隔离凭证和响应、旧请求晚返回不污染新任务、刷新恢复、账号/租户切换、租约过期重新领取。结果未知时必须保留原 key 和原参数，不能通过换参换 key 绕过待确认操作。

### P2：规范有文字约束，尚无依赖方向检查

证据：[Go lint 配置](../.golangci.yml)、[ESLint 配置](../web/eslint.config.js)、[CI](../.github/workflows/ci.yml)。现有检查覆盖常见 Go 错误、基础命名和前端语法/类型，但没有模块依赖规则。代码审查仍承担防止跨层访问和业务反向依赖 Demo 的工作。

后续先列出现有合法依赖和例外，再增加少量明确的 import 禁止规则，验证允许的导入通过、禁止的导入失败。复杂度阈值先采样，不能把全体 lint 改成“仅查 diff”，否则原有正确性检查也可能被过滤。`errcheck` 对所有包含 `Close` 的诊断豁免较宽，修改资源写入路径时应复核关闭/刷新错误是否影响结果，不为排版批量改写。

### P2：事务规则与现有例外曾相互冲突（文档已修正）

证据：[架构说明](architecture.md)、[user.go](../internal/modules/system/repository/user.go)、[role.go](../internal/modules/system/repository/role.go)。用户创建/更新/删除和角色删除在 Repository 内持有短事务，维护用户角色关联的原子性；架构文档已认可这项例外，旧迭代计划却要求作为“无行为变化的护栏”迁到 Service。

本轮将规则统一为：库存等跨模块用例由 Service 持有事务；保留 system 聚合短事务例外。将来若确有组合用例需要复用事务，再独立迁移并验证角色分配/删除竞争、回滚和权限缓存失效。禁止只为分层统一拆掉现有原子边界。

### P2：启动建约束的错误处理不足，路线图不能只写“换错误码”

证据：[AutoMigrate](../internal/bootstrap/database.go)、[启动入口](../cmd/wms/main.go)。开发路径执行 CHECK DDL 时通过错误文本判断重复；其他 DDL 错误也只记警告，函数最终返回 nil。release 路径不调用 AutoMigrate，也没有在此入口校验 CHECK 是否已安装。

后续需验证实际 MySQL 约束错误与结构元数据，区分“已存在且正确”和“建立失败”；不要沿用旧文档中未经验证的 1061 假设。验收至少包括首次创建、重复执行、同名但定义不符、历史脏数据/DDL 失败与 SQL 迁移往返。此项涉及启动行为，应作为独立正确性修复。

### P2：规模假设和热点文件需要明确边界

- [TaskAPI](../internal/modules/task/api/api.go) 的 `DetailTaskPageSize=200` 被[出库详情](../internal/modules/outbound/service/query.go)用于第一页查询且丢弃 total；[PickDialog](../web/src/components/PickDialog.vue) 依赖详情任务列表。超过 200 个任务时，不能从这个弹窗看到全部任务。应采用独立任务分页或明确暴露截断信息，不能只调大常量。
- `pick.go` 431 行，包含拣货、领取、扫描校验、快照和重放；`OutboundOrders.vue` 530 行，脚本同时处理查询、批量操作和创建表单。行数只用于定位热点，不是质量判决。修复业务缺口后，可按稳定职责拆文件/组件，保持一个事务回调可连续阅读，不增加通用工作流框架。

### P2：测试待办应按场景列出，不能按文件名推断缺失

[import_cleanup_test.go](../internal/modules/inbound/service/import_cleanup_test.go) 已验证失败文件过期与孤儿文件清理；[import_test.go](../internal/modules/inbound/service/import_test.go) 已验证执行权、恢复 PENDING 和退出；拣货测试位于 `internal/app/pick_race_test.go`，并非没有测试。盘点也已有锁等待和回滚测试。

仍值得补充的明确场景：上架多步写入后的失败注入与整体回滚、真实 1213 的整事务重试与耗尽、PDA 前述操作状态用例、导入源文件缺失后的 worker 终态。新增测试应断言业务不变量，避免只断言私有函数被调用。

## 4. 扩展时的约束

| 扩展目标 | 推荐起点 | 必须保留/补充 |
| --- | --- | --- |
| 新增查询、筛选或字段 | 所属模块 DTO、查询、前端 API 类型 | 精确租户范围、分页 total、字段兼容性与响应测试 |
| 退货、调拨等库存流程 | 独立用例与明确的单据状态机 | 库存变更与流水同事务；跨模块复用同一 tx；补取消、重试、回滚测试 |
| 收货/上架请求级幂等 | 已有 `pkg/idempotency` 与客户端操作状态 | 每次真实操作独立 key；原参数重试；记录与业务同事务提交；不按相同内容猜测重复 |
| 多实例 | 先明确部署需求 | 共享导入存储、节点号唯一、权限失效/限流范围、worker 执行权、跨实例恢复验证 |
| 大单/多 PDA | 先补任务分页与指标 | 量化行锁等待、事务重试和规模；保持现有锁协议；不凭文件长度改锁粒度 |

## 5. 本轮文档变更与验证

本轮更新评审报告、迭代路线、工程约定、导航、需求中的相关语义，以及本地交接/历史记录入口。将“现状”“后续计划”“历史验证”分开；不把文档修改标记成业务修复完成。

| 文件 | 修改目的 |
| --- | --- |
| `maintainability-review.md`（新增）、`iterations.md`、`go-style.md` | 评审依据、任务优先级/验收条件与长期规则 |
| `README.md`、`code-index.md` | 文档导航与已有测试入口 |
| `requirements.md`、`development.md` | 发货/幂等语义与本轮验证范围 |
| `context-handoff.md`、`review-progress.md`（本地忽略） | 当前交接事实与历史记录边界；保持不提交 |

验证结果见 [开发与验证指南](development.md) 的 2026-10-10 记录。数据库或 Redis 测试跳过必须单列，不能用命令退出码 0 证明并发链路已复验。race、E2E、压测与部署需要各自独立证据。
