# 文档目录

[返回项目首页](../README.md)

## 使用与部署

- [本地开发与验证](development.md)：开发环境、测试依赖、E2E 和交付打包。
- [部署与升级](deployment.md)：Windows / Docker 启动、迁移、账户、HTTPS 和自动部署入口。
- [配置参考](configuration.md)：常用环境变量和 Compose 账户配置。
- [演示指南](demo.md)：体验流程、演示账号与会话边界。
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
