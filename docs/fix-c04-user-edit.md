# C04 第三部分：用户编辑版本与凭据版本分离

对应I5用户管理实例，基线2e0cc35，2026-11-03实施。与035ff7f（D4/I34）及2e0cc35（I5入库）合并构成C04代码修复；浏览器验收仍待补，不记为全栈验收完成。

## 根因与实现

旧用户编辑窗口即使只改昵称，也提交打开时的status和role_ids。另一管理员停用账号并撤权后，旧窗口仍能恢复旧状态和角色。另一方面，任何带status的保存都递增token_version，状态未变的昵称编辑也会使被编辑者退出。

迁移000018给sys_user增加独立version，默认1，供编辑并发控制。用户列表返回version，完整编辑必须携带expected_version。Service在C02已锁定的目标用户上比较后才写资料/角色，不匹配返回40902。资料、角色关联、密码/状态以及受管体验账号同步都会推进编辑版本；昵称或未改变的状态不会推进登录撤销版本。显式状态命令只处理目标状态，相同状态不重复写或踢人，不携带旧昵称/角色。

前端绑定打开时的修订号，冲突后保留原输入、刷新列表供重新打开核对，不自动换新版本重发；旧列表缺版本时禁止编辑。原角色选项失败保护和权限委派校验保留。

## 文件范围

- `migrations/versions/000018_user_edit_version.{up,down}.sql`、`system/model/model.go`：独立编辑版本。
- `system/{dto/dto,handler/handler,service/user,repository/user}.go`：版本契约、锁内核对、显式状态命令及全部用户写入推进修订号。
- `bootstrap/{account_credentials,demo_accounts,personal_accounts}.go`：初始化/同步也参与编辑版本；原凭据CAS再核对该版本。
- `web/src/api/types/system.ts`、`web/src/views/system/{Users.vue,Users.roles.spec.ts}`：编辑版本、冲突保留与回归；既有HTTP/E2E夹具补必填版本，未削弱权限拒绝断言。
- 新增`internal/app/user_edit_test.go`、`system/service/user_edit_test.go`、`migrations/user_edit_test.go`；更新原用户与凭据测试。API、架构、部署、约定和索引同步。

## 验证与已知波动

正式迁移MySQL+JWT+HTTP回归在修复前确认：撤权后旧窗口保存仍200/code0。修复后拒绝并保持停用、空角色、密码版本及昵称；刷新后正常编辑、密码重置导致旧窗口过期、无变化状态不踢人、新登录正常。两名并发编辑者仅一个成功。迁移17→18→17→18保留现有/软删账号、密码、登录版本与角色关联，新增版本默认1；开发与正式结构契约均通过。

Go最终：gofmt、go vet ./...、go build ./...、git diff --check通过；全量50个有测试包820项含子测试通过，0失败/测试跳过，29包无测试。专用MySQL23309、Redis26389；WMS_TEST_REQUIRED=1。

前端专项48项、lint、vue-tsc/生产构建通过。首次与Go全量同时运行的前端全量在既有依赖规则beforeAll超过10秒，导致27项未执行，其余462项通过；原日志保留在user-all-frontend.log。Go结束后单独运行同一npm run test，42文件489项通过、无跳过，日志user-all-frontend-isolated.log。没有改变超时/断言/并发配置；这只能证明本次隔离复核通过，不能关闭H01/Q5或断言负载就是根因。

本组日志位于本地忽略目录tmpgen/c04-fix-20261102/user-*。未运行race或生产验收。前端预览启动此前被自动审批拒绝，只返回blocked by policy，未绕过；本组浏览器验收仍未执行，不能用HTTP/组件状态测试代替。

## 发布边界

先停旧后端写入并备份，迁移到18，再同步发布前后端并刷新旧编辑窗口；不能混跑不推进版本的旧后端。旧客户端缺少expected_version返回400。回退需停新应用并使用匹配旧版本，重新升级后原窗口必须重新加载；本次只验证隔离测试库，不执行生产迁移。

C04覆盖已登记的入库草稿与用户编辑、改密及体验凭据撤销，未承诺所有CRUD都具备编辑保护。角色编辑页面问题仍属M07，盘点录入冲突属C12，多查询一致快照属C10；C03认证会话竞争接续处理。
