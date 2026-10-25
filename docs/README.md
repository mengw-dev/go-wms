# 文档目录

[返回项目首页](../README.md)

## 使用与部署

- [仓储操作手册](warehouse-operations.md)：从基础资料到收发盘点、移库、扫码和未知结果核对。
- [基础版收尾验收](release-acceptance.md)：交付范围、本地联调、分支状态与上线前核对。
- [隐藏问题定向验收](hidden-audit-20261024.md)：权限组合、离页确认、盘点交错、流水失败和领取过期的复现与修复。
- [操作恢复专项验收](recovery-audit-20261024.md)：异常回执、弹窗内原请求恢复、波次权限与选项重试。
- [锁粒度与逻辑修复](locking-logic-audit-20261025.md)：波次锁顺序、仓库/SKU共享引用保护、当前分配锁读及定向并发回归。
- [业务流程与操作入口验收](business-workflow-audit-20261025.md)：200项历史场景、真实业务组合发现、F36–F38修复及本轮回归范围。
- [单实例备份与恢复](backup-recovery.md)：人工备份、隔离恢复和数据完整性检查。
- [本地开发与验证](development.md)：开发环境、测试依赖、E2E 和交付打包。
- [部署与升级](deployment.md)：Windows / Docker 启动、迁移、账户、HTTPS 和自动部署入口。
- [配置参考](configuration.md)：常用环境变量和 Compose 账户配置。
- [演示指南](demo.md)：体验流程、演示账号与会话边界。
- [波次与播种拣货](wave-picking.md)：同仓组波、汇总拣货、软件槽位分货、逐单确认发货及异常边界。
- [异常、冻结与客户退货](warehouse-closure.md)：操作入口、库存约束、恢复与验收。
- [仓内移库](stock-movement.md)：同仓库可用库存移动、原请求恢复与流水追溯。
- [库存预警](inventory-alerts.md)：规则配置、缺货/低库存与恢复口径。
- [扫码辅助作业](scan-assistance.md)：收货定位明细、上架选择库位、拣货核对，扫码不自动写库存。
- [单据打印与标签](document-printing.md)：打印入口、标签规格与实机边界。
- [Excel导出](excel-export.md)：筛选、行数上限、字段格式与盘点差异。
- [示例数据](../samples/README.md)：Excel 导入与外部系统推送样例。

## 业务与实现

- [需求与已知边界](requirements.md)：功能范围、已知限制与当前迭代顺序。
- [迭代记录与路线图](iterations.md)：已完成的修复、待完成的迭代事项与工程约定。
- [架构说明](architecture.md)：调用关系、事务、库存、异步任务与租户。
- [数据库设计](database.md)：表、索引与迁移。
- [引用完整性取舍](database-design.md)：应用层引用检查的职责与边界。
- [API 文档](api.md)：认证、权限、字符串 ID 与业务接口。
- [Go 约定](go-style.md)：命名、依赖、错误、事务与测试。
- [代码索引](code-index.md)：按模块查找实现和对应测试。

## 验证与运行

- [压测指南](load-testing.md)：k6 业务场景、压力模型和结果解释。
- [监控说明](monitoring.md)：Prometheus、Grafana 与指标查询。
- [生产就绪评估](production-readiness.md)：现有能力和运维缺口。

文档修改应与代码同步；历史测试记录不等于对当前提交重新验证通过。新增能力优先更新对应专题，项目首页仅保留摘要和链接。
