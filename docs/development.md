# 本地开发与验证

> 状态与证据口径：下方各日期/阶段为当次快照，测试数量、未运行项、提交与部署描述只对该基线成立。历史“未提交”不是当前待办；当前登记见 [问题总表](review-findings.md)。本地验收、Git 提交、推送、CI 和目标环境上线分别确认，本轮文档核对不重跑历史业务测试。

## 2026-10-21 D25 单据规模、锁竞争与查询计划（仅本地）

在e187287基础上只提取本轮改动形成独立验证快照，原外部订单来源/查询/保护、迁移000013及扩展计划未纳入。新增默认200明细/1000累计分配行预算，所有租户生效，配置与内部构造均兜底；审核读取上限加一条明细，分配越界前停止，事务整体回滚。手工/外部新单提前限明细，降低配置后的历史同号回放与取消仍可用，超长伪回放不进入逐条SKU解析。FIFO/数量上限/锁协议不变，没有新增HTTP字段或前端类型。

真实本地MySQL8.0.46：6种规模各5次，最大组合200明细/1000分配行的中位1141.959ms、最大1276.746ms；逐单确认分配/任务/流水数量及业务取消后守恒。预算边界、跨明细累加、连续拒绝零残留、提高预算后重试、历史审核拒绝及外部回放通过。锁门用例从performance_schema观察真实同仓等待，分仓可完成；8路既有四元组Increase验证通过，缺失四元组的初测观察1213后重试成功，不把所有竞争都归仓库门锁。正式迁移1万条混合库存完成首批/续页EXPLAIN、ANALYZE及真实分页，目标100条命中索引后排序，不强制选特定索引。条件、全部数据及复现见[规模验证](scale-validation.md)。

独立快照最终gofmt、go vet ./...、go test ./... -json -count=1通过：47个含测试包、520项含子测试、0失败/0测试跳过，另31包无测试文件；go build ./...通过。全仓lint首轮发现测试助手context参数顺序和候选快照格式问题，已修正；修正后全仓lint为0 issues，锁门/并发专项连续三轮通过。无web变更，不重跑前端/浏览器；未跑race、HTTP容量、容器/监控栈或生产压测、CI，未查询远端。不是所有数据分布的时延或吞吐保证；同仓串行协议、生产容量B16与持续索引维护D27保留。

专用MySQL23306与Memurai26379已关闭，无生产或远端操作。提交前84处本地文档链接通过，暂存Go/配置与验证快照一致；以独立Git索引还原原五文件补丁，SHA256仍为c554d1987d7f296f64e3f4bf16454929f2bc710c41f1d60f137ce7a27dabf2ed，并核对原文件施加本轮变换后与工作树字节一致。原123增/4删仍留在工作区，扩展计划四行不混入暂存。

D25在本地有界实施范围完成；D工程项余1（D27），不是全项目只剩1个问题，B类边界、功能计划与全仓未补齐审查证据仍保留。只做中文本地提交，不推送GitHub。

## 2026-10-21 D10 文档状态与历史验收口径（仅本地）

核对源码基线0e23f68及本地提交记录，集中整理 iterations、requirements、production-readiness、ui-review、review-findings 与本文件；本地交接/持续审查/旧维护性报告增加历史快照说明，忽略文件不强制提交。统一由 review-findings 维护 F/D/B 状态，迭代文件引用该入口；旧日期中的剩余数、未提交、测试数量只表示当时快照，实现、本地验收、提交、推送、CI、上线分开确认。

纠正 M2 依赖护栏待实现/计划 depguard、D26 导入源文件丢失与进程恢复/业务指标仍待补、PDA 已修复生命周期被重列为当前缺陷、跨 key 内容去重被当作必修漏洞等表述；补 D11/D12/D13 后续加固、真实前端持久化字段与 tenantId 未填边界、外部单 SKU ID 快照口径。需求中的前端菜单权限与代码对齐，盘点状态改为模型实际的 COMPLETED，100ms 列表时延明确为待规模验收目标，完整验收不接受无依赖自动跳过。UI/生产就绪区分配置存在、历史夹具/真实业务回归和目标环境部署，不宣称最新提交已在线通过。

只做文档核对：10项语义检查通过（含源码存在/状态/字段与剩余D项），文档本地链接与diff检查通过，原README/业务扩展计划字节与原外部单五文件diff哈希保持不变。iterations存在原计划未提交段，本轮生成基于HEAD的独立暂存补丁，仅纳入D10订正，原计划继续留在工作区；未把原未提交外部单扩展当作已交付能力。资料在tmpgen/document-status-20261021。

没有改Go/web/迁移/配置/脚本，没有重跑后端集成、前端、浏览器、race、CI或查询远端部署，也未启动数据库/缓存/预览服务。此前497项Go、287项前端或23项清理脚本通过属于各自日期和代码基线，不是本轮重新运行的结果。此次是状态与已发现矛盾的订正，不等于完成所有API字段/源码的逐行复核，D27继续跟踪。

D10完成，当前工程项仍开放D25受控规模/锁竞争/EXPLAIN及限额决策、D27持续维护和剩余审查证据；B类能力/运维边界保留。仅中文本地提交，不推送GitHub。

## 2026-10-21 D22 压测清理脚本范围与退出码（仅本地）

原未跟踪的 cleanup-allocation.js 仅按模糊 K6PROOF 关键词取最多十页订单、库存只看首100行，批量失败及 check 红灯不能保证 k6 非零退出。本轮先备份原文件到 tmpgen/allocation-cleanup-20261021/cleanup-allocation.original.js（SHA256 0187d21f989bee2b59728ed0b858fde770fc843643fc1bccc0f2aab5faf0fe8f），再完成该脚本并加入两套回归；其他原未提交业务功能/计划保持独立。

必填仓库/SKU与显式账号，字符串int64保持大ID精度；从登录token的原始租户数字文本检查正租户，拒绝平台0，服务端JWT鉴权不变。默认只读，CLEANUP_EXECUTE=1才写；精确前缀/租户/仓库、逐单全明细单一SKU及未拣货检查，混合货品整单排除。完整订单/库存分页检查total稳定、列表长度、重复与100000行预算，超限失败而非截断。只在总占用与候选活动分配一致时允许取消；批次至多200，取消前复读固定顺序业务字段，检查批次计数/错误、逐单终态、现存不变/分配清零/可用回补。未知结果或部分失败非零退出，不自动重发或撤销此前已成功批次。现有ID取消接口无法原子包含客户端范围条件，停止同范围其他写入仍是前置条件；不宣称支持并发编辑期间的范围锁。

最终三文件 node --check、13项VM隔离回归和10项真实k6回环HTTP回归通过（共23项，0失败/0跳过），k6 v2.2.0。覆盖1101张订单/12页/6个取消批次、101库存行、大ID与大租户、默认无写入/重跑、其他范围及混合SKU、缺页/重复/总数变化、已拣/已发货/数量缺失/其他占用、预写变化、后批失败保留前批成果；真实进程预览/执行退出0，必填参数/HTTP500/非JSON/断网/登录解析/部分取消/响应未知/库存不回补均退出108且不盲目重试。没有连接WMS、MySQL、Redis或执行真实业务取消。

首次真实k6测试因本机v2已移除 --no-summary 参数失败，测试去掉该可选展示参数后继续；真实JSON对象属性枚举暴露完整JSON.stringify误判变化，改为固定顺序业务字段并补键顺序回归后重新完整验证23项通过，未放宽失败规则。回环夹具在测试结束关闭。无Go或web改动，不跑后端集成/前端/浏览器/race/CI或生产压测；这些脚本回归不证明真实取消事务的库存释放正确性，服务端既有回归仍独立。

D22完成，余D10/D25/D27共3项；使用方式与边界见load-testing.md。仅中文本地提交，不推送GitHub。

## 2026-10-21 D08 资源关闭错误与静态检查（仅本地）

移除 .golangci.yml 中对所有包含 Close 文本的 errcheck 诊断豁免，保留原有测试文件例外。审查非测试 Go 资源关闭点：现有上传落盘使用 os.WriteFile，标准库已传播写入及关闭错误；漏洞扫描报告关闭错误、服务/数据库/缓存退出已有处理，本轮不改这些既有逻辑，也不把维护债描述成已存在的业务写入缺陷。

multipart 文件只读，读取失败由 ReadAll 返回；监控 SQL 游标正常耗尽时自动关闭并检查 rows.Err，提前 Scan/解析失败已使采集失败。这两处用有理由的显式兜底关闭，避免释放错误覆盖原读取/采集结果。Excel 工作簿的解析临时文件及导入清理目录关闭失败改为带 task_id 的告警；保持逐行已提交单据、导入计数/状态和原失败，不因清理失败诱导重放写入。现有 .env、HTTP 请求/响应 body 的显式只读关闭及测试基础设施释放保留；不机械改写所有关闭点，不新增通用资源框架、依赖或迁移。

基线 88bd221，只叠加本轮 4 个 Go 文件和 lint 配置到独立候选，排除原未提交外部单、000013迁移及业务扩展计划。用真实 golangci-lint 对标准库文件/缓冲写入夹具验证：旧配置漏报未检查 Close、仍报告 Flush；新配置同时报告两者，检查错误和有意忽略只读关闭的夹具通过。最初夹具把两处错误写同一行，被诊断按行去重后导致断言不足；分行后按原规则验证通过，没有放宽检查。夹具仅在 tmpgen 保留，未添加只为日志分支而注入的业务测试接口。

导入解析/资源清理、导入查询参数和真实 MySQL 行锁指标专项通过；最终 gofmt、go vet ./...、go test ./... -count=1（47 包、497 项含子测试、0 失败/0 测试跳过；31 个包无测试文件）、go build ./... 和全仓 golangci-lint（0 issues）通过。实际 Excel Close/目录 Close 失败日志没有进行底层文件系统故障注入；已有解析/清理失败和业务状态回归通过，不宣称告警后临时资源一定清理成功。MySQL23306、Redis兼容Memurai26379为专用本地测试服务，验收后关闭。无前端变更，不重跑前端/浏览器/race/CI/生产压测。材料 tmpgen/resource-close-20261021。

D08 完成；D 工程项余 D10、D22、D25、D27 共 4 项。仅中文本地提交，未推送 GitHub，原未提交内容保留。

## 2026-10-21 D13 无歧义请求指纹与旧记录兼容（仅本地）

原 Fingerprint 将字段用 `\x1f` 连接后哈希，字段中含此字符时不能通用证明边界。本项属于设计约束加固，尚未证明原有受限业务参数可以触发可利用碰撞。新指纹把字段数及各字段字节长度（大端 uint64）与原始字节一起编码，经 SHA-256 后存储为 `v2:` 加无填充 base64url，共 46 字节；现有 VARCHAR(64) 足够，无新依赖或迁移，不使用可能替换非法 UTF-8 的文本序列化作为输入。

收货、上架、盘点审核、领取及拣货的新写入统一保存新格式；快路径和事务回滚后的核对统一调用 RequestFingerprint.Matches。固定字段数、所有字段均不含分隔符的旧 64 字符十六进制记录仍按原 key 回放，旧记录不重写。每个 scope 的字段顺序、参数参与范围与原有规范化不变（收货批次精确比较，拣货扫码沿用 trim/小写、保留入口区分，领取凭证不加入拣货指纹）。事务位置、锁顺序、租户/scope 唯一键、响应快照与前端存储规则未修改。

含分隔符的旧记录无法通用地确认边界，返回既有 40901 阻止回放，客户端保留原 key；必须核对业务结果后处理，不能删除记录或换 key 重发。新格式记录允许此类内容并可正常回放。旧服务不能识别新指纹，混合版本/回滚期间可能拒绝 v2 回放，发布需协调实例版本；本轮没有部署或触碰生产数据。

基线 9661f58，只叠加本轮 9 个 Go 文件到独立候选，排除原未提交的外部单功能、000013 迁移与业务扩展计划。新增 11 个含子测试 pass 事件，覆盖确定的编码向量、空列表/空字段/分隔符/字段移位/中文/非法 UTF-8、旧格式安全兼容与歧义拒绝；五类命令验证旧记录回放，收货/上架/审核核对分支及领取/拣货首次查询未命中后的回滚核对或事务重试回放；检查库存/流水/任务不重复、领取凭证不轮换、旧快照不重写。字段移位使用保存记录夹具，不证明这些扫描内容通过现有业务校验。

最终 gofmt、go vet ./...、go test ./... -count=1（47 包、497 项含子测试、0 失败/0 测试跳过；另 31 个包无测试文件）、go build ./... 与全仓 golangci-lint（0 issues）通过。MySQL23306 和 Redis兼容 Memurai26379 均为专用本地测试实例，验收后关闭。无前端变更，不重跑前端或浏览器测试；未运行 race、CI、容器或生产压测。材料 tmpgen/fingerprint-20261021。D13 完成、剩余 D08/D10/D22/D25/D27 共 5 项；仅中文本地提交，不推送 GitHub。

## 2026-10-21 D11 前端幂等公共机制（仅本地）

新增composables/operationLifecycle.ts，四套composable复用当前身份/存储选择、按身份与对象生成状态键、prepared→sending提交前持久化、错误分类/错误消息与Vue作用域内storage监听。收货/上架/盘点审核/PDA领取/PDA拣货的safe-reject策略按五个scope分别集中维护，与HEAD原数字集合及顺序逐一对照保持一致；网络/401/403/5xx/未知及其他命令错误码不能据此清记录，40901保留并阻止继续提交。

业务状态Map、payload精确/归一化比较、凭证/租约、请求序号、发送/恢复/放弃与成功清理流程继续在各自composable，不建立通用工作流。存储键/版本/内容与7天窗口不变，不增加跨标签页锁、不修改后端API或共享请求拦截器。新公共函数不直接发业务请求，身份每次使用重新读取，不把await前身份作为当前身份。

基线2d664fc。新增37项跨命令契约与公共机制回归，五个scope均覆盖结果未知保留原key/参数、按命令安全拒绝后换新key、40901阻塞、prepared/sending任一步保存失败不发送、旧账号响应不清除新账号记录且不清除新请求loading、Vue owner销毁注销同一个监听；另覆盖普通/演示存储、访客切换、大字符串ID和未知错误形状。最终npm run lint、25文件287项单测（0失败/0跳过）、vue-tsc与Vite build通过。首次build指出上架接口类型缺Record索引签名及测试异型回调泛型推断问题，分别显式适配既有业务payload类型与包裹回调后重跑完整前端校验通过，未放宽类型或lint规则。

Chromium业务布局fixture4项通过（白天/夜间手机选择/分页/详情/收货入口，以及8类新增表单、收货/上架/PDA拣货窗），零重试；接口只读fixture用于页面可用性，不证明真实业务写入。没有后端/数据库变更，不运行Go/MySQL/Redis集成、真实写入E2E或CI。材料tmpgen/operation-lifecycle-20261021，验收后关闭Vite25179。D11关闭、工程项余6；仅本地中文提交，不推送，原未提交业务功能与计划保留。

## 2026-10-21 D12 幂等过期清理（仅本地）

原清理一次DELETE全部过期记录，现每批按created_at/id选择最多500主键，再重查过期条件删除，保留GORM租户过滤和严格早于截止时间的边界。保留期7天、后台启动立即执行；每批独立提交，批间20ms，每轮最多20批，每批查询和删除共2秒，整轮30秒或调用方更早截止。达到批数上限/发生错误后等待min(正常间隔,1分钟)继续；确认清完后等待原24小时。错误与取消不撤销先前批次，不把整轮包进长事务，不新增迁移/依赖/组件。

复用Metrics记录后台每轮completed/limited/error/timeout/canceled耗时，已确认删除行数、成功分批DELETE数和最后未触及上限的成功时间；nil Metrics仍可正常清理。Grafana新增3个面板，JSON/22个唯一面板ID与网格无重叠检查通过。行数是收到成功响应的进度，不等于网络结果未知时的精确数据库物理变化；limited仅表示达到上限，不代表已精确计量积压。Purge直接调用不产生后台轮次指标，API注释与monitoring.md已同步。

基线2ebe78b加本轮4个Go文件的独立快照，排除原外部单和000013迁移。gofmt、go vet ./...、强制真实依赖go test ./... -count=1 -json -timeout=180s、Go build全过：47包、486个含子测试pass、0fail、0skip。真实本地专用MySQL23306、Redis兼容Memurai26379。新增清理回归覆盖1007条多批且保留临界/新鲜/未知时间、10001条上限与跨轮继续、普通租户及精确0、并发重复选中不重复计数、查询与删除失败、2秒真实行锁超时与1秒更早调用方deadline、刷新过期时间后二次校验、部分进度/错误/超时/取消指标与退出。

最初锁等待夹具预设先提交第一批，但实际首批已等待锁，未满足预设第二批场景；改为明确锁住首批目标验证真实超时，前批保留由第二批故障/取消回归独立验证，不把特定执行计划当作断言。首次lint指出测试故障注入AddError返回值未显式丢弃，已修为保留GORM错误状态的调用；修后清理专项16个含子测试与包级vet再次通过、全仓lint0issues，未更改全仓规则。完整运行与最终专项材料在tmpgen/idempotency-cleanup-20261021。

本轮无web源变更，不重跑前端/浏览器；未运行实际Grafana/Prometheus采集渲染、容器、race、生产规模压测或CI。测试服务结束后关闭。D12关闭，工程项余7，D25规模验证保留。仅中文本地提交，不推送；其余原未提交业务文件与扩展计划保留。

## 2026-10-21 D09 过期注释（仅本地）

按实际调用链更正幂等包使用约定：收货/上架/盘点审核在事务外预查询，拣货/领取保留事务内快路径；Find 为普通读取，不能强制看到并发提交，命中仍须核对指纹和结果，业务与Insert同事务，必要的核对在回滚后新读且查询失败保留结果未知，持事务连接时不另借连接。空成功写入固定"{}"与读取允许空白的空对象校验分别说明。000001顶部与database.md统一为生产版本化迁移、开发/测试AutoMigrate，历史F12建议同步完成状态，不改任何SQL语句。

基线f4720f7。gofmt差异为空；标准库parser验证Go语法，scanner排除注释后所有token与HEAD一致；SQL去除开头注释区并归一化换行后正文与HEAD一致；go vet ./internal/pkg/idempotency通过。文档链接与git diff --check另行检查。材料tmpgen/d09-comments-20261021。本轮仅注释/文档，不新增测试，不运行数据库迁移、MySQL/Redis集成、前端、浏览器、全仓业务测试或CI，也不把上一轮470项结果当作本轮重跑。

D09关闭，D工程项余8；D10其余文档状态漂移仍保留。按用户要求仅中文本地提交，不推送。原有外部单、000013迁移/脚本/简历和之前业务扩展计划仍保留，不纳入本次提交。

## 2026-10-21 D26 导入恢复与业务观测（仅本地）

导入源文件丢失时明确 FAILED，并提示先核对已生成单据，避免把丢文件当作可直接整份重传；不泄露源路径。保留原有逐行幂等、run_token 与心跳协议，新增真实子进程中断回归：第一行提交、第二行事务进入建单前由父测试强制 Kill 子进程，不执行 defer/优雅释放。新服务实例先验证新鲜心跳不可回收，再只回退数据库心跳时间模拟等待超过5分钟，补偿并重跑后共三行且第一行 ID 不变；源文件同时丢失的变体只保留原已提交单据、明确失败，旧 token 不得覆盖结果。任务失败后的零行计数不代表历史没有部分建单，排查须按批次核对。

沿用现有 Prometheus/Grafana，补齐事务尝试耗时、实际额外重试、导入执行结果/耗时、过期恢复次数、全租户队列与最早待处理年龄，以及 MySQL 实例行锁等待；固定低基数标签，不带租户/单号/路径。队列/行锁按抓取查询，2秒预算，失败单独报 scrape_success=0 并省略数据，避免假零。队列 SQL 排除软删除，MySQL 指标与连接池等待分开，多应用副本共享同库不能相加。Grafana 新增9个面板，口径与恢复操作见 monitoring.md。事务指标覆盖 tx.Manager，完整请求耗时沿用原 HTTP 指标，不声称覆盖全部直接 SQL 或提供 D25 压测结论。

静态检查指出成功任务文件删除缺少目录边界，现以 filepath.Rel/IsLocal 和 os.OpenRoot 限定在配置上传目录中执行删除。回归覆盖目录外文件保留、路径不丢失、目录已不存在仍清理持久路径，以及原成功残留/失败重试。测试辅助进程只启动当前测试可执行文件与固定参数，G204 仅在此行注明依据；没有放宽全仓规则。初次 lint 的不变参数及路径告警已修正，未按旧告警直接忽略。

验收基线为已提交 3fd0ca3 加本轮15个后端源/测试文件的独立快照，排除原未提交外部单及000013迁移。gofmt、go vet ./...、强制依赖 go test ./... -count=1 -json -timeout=180s、Go build、golangci-lint 全部通过：46包、470个含子测试 pass、0 fail、0 skip、lint 0 issues。仅使用本地专用 MySQL23306、Redis兼容Memurai26379。新增回归还覆盖普通/精确租户0/软删队列聚合、空队列、采集失败省略数据、实际 MySQL 持锁等待、重试成功/耗尽/1205不重试/取消/恐慌指标，以及部分导入/正常取消归还的结果指标。

Grafana JSON、19个面板 ID 与网格无重叠验证通过；未运行 Grafana/Prometheus 实际渲染及采集、生产代理、容器、race或云端CI。没有前端变更，不重跑前端测试。材料 tmpgen/import-reliability-20261021，结束后关闭本轮专用服务。D26关闭、D工程项余9；B03本地文件共享边界与D25压测/规模限额仍保留。按用户要求只做中文本地提交，不推送；其余计划及原未提交业务文件不纳入。

## 2026-10-21 D20 / D21 AI 可靠性（仅本地）

分钟/日额度的 INCR 与过期设置改为单键 Lua，并修复旧的无 TTL 计数；有效窗口不随请求延期，Redis 故障时仍按原有策略放行。新增整次问答预算25秒，含 Redis/快照/主备请求；主模型有备用时最多使用剩余总时间一半，取消或总超时不再启动备用，401/403不切换。服务端写超时/默认Nginx读超时30秒、AI浏览器35秒；单次模型上限仍配置为30秒但受整体/尝试上下文约束。配置和API说明已同步。

前端显式传 AbortSignal，提供停止等待，离页/会话变化取消并屏蔽旧答案；问答错误在对话中保留服务器原因，取消不弹网络错误。没有更换模型、增加中间件或修改库存业务/迁移。调用模型仅用本地 HTTP 替身，无生产模型请求。

从已提交 HEAD 导出快照，仅覆盖本轮6个后端/配置文件，排除原未发布外部单代码。Go vet、46包458个含子测试 pass/0 fail/0 skip、golangci-lint0issues、build通过；真实专用 MySQL23306 与 Redis 兼容服务 Memurai26379。专项覆盖并发阈值/租户与用户范围、原两步调用被取消留下无TTL的复现与修复、脚本执行后响应丢失、窗口不延期、主备共用预算、401/403、无备用/相同模型、预取消、MySQL检索超时以及来访HTTP断开传递至模型HTTP上下文。首次HTTP夹具因未消费请求体阻塞关闭、配置夹具缺port已修正，最终完整回归通过。

前端最终lint、24文件250单测、vue-tsc/build通过；10项Chromium回归通过，包含日夜手机320/390px停止等待、离页取消、旧答案不覆盖新问题、429日额度提示，以及原有手机作业/弹窗/AI输入布局。UI接口拦截验证与Go双HTTP/真实依赖验证分开记录，未运行完整真实业务浏览器链、Docker/实际Nginx、race或云端CI，未将默认时间预算契约当成生产环境实测。

材料在 tmpgen/ai-reliability-20261021；本轮结束后关闭专用服务。按用户明确要求只做中文本地提交，不推送。D20/D21关闭，D工程项余10；D10其它漂移、D26导入恢复与观测仍保留。先前业务扩展计划及原未提交外部单等均保留，不纳入本次提交。

## 2026-10-21 手机业务页面适配（仅本地）

手机截图暴露出筛选占用高度、分页下一页单独换行、横向表格隐藏操作以及演示控制条过大的问题。本轮仅改前端呈现：业务列表/详情/收货表格在 768px 及以下使用原单元格显示纵向字段卡片，保留原权限、选择和业务处理；公共筛选可折叠，手机分页固定分组排布，表单标签与输入纵向排列，弹窗内容内部滚动、确认/取消固定。公共演示浮条在手机保留状态、控制和拖拽，重复重置入口仍在控制抽屉。详情见 [UI 记录](ui-review.md)。

前端最终 lint、24 文件 / 249 单测、vue-tsc 与生产 build 通过。页面回归覆盖 48 项：业务布局 26、仪表盘 9、演示首页 4、演示结果滚动 4、工作区 5。前 43 项在组合运行中已逐项完成；最后工作区仍采用“横向表格/直接展开筛选”的旧断言，停止该旧运行，按手机卡片和折叠筛选的新行为更新后，工作区 5 项单独运行全部通过。新增矩阵覆盖日夜主题下 320/390/430/768px 的全部 11 类业务列表、字段标签与完整操作、局部滚动/分页对齐和默认悬浮条避让；另验批量选择/清除、翻页请求、进入详情、收货输入可达、8 类新增窗和日期选择器。开发中也修正了卡片宽度/标签样式优先级及测试日期定位。初始失败记录保留在 tmpgen/mobile-business-20261021，未用重试掩盖失败。

UI 使用拦截接口数据，验证呈现与事件转发；未运行真实后端写入链或实体手机浏览器测试。无后端/数据库/依赖变更，未重跑 Go / MySQL 集成测试。已人工查看日夜截图，材料在 tmpgen/mobile-business-20261021/screenshots。

按最新用户要求只提交本地 Git，不推送、不运行云端 CI/部署；上一项库位卡片修复 c95f203 同样仍仅在本地。原外部单功能、迁移、脚本和其他未提交文件不纳入本次提交。临时 Vite25178 在验收后关闭。

## 2026-10-21 库位占用率卡片点击修复（仅本地）

仪表盘原先在“全部仓库”时禁用库位占用率卡片，但仍显示跳转箭头。后端库位查询原本支持不传仓库条件，本轮仅修改前端：全部仓库与指定仓库均可点击卡片，跳转库位页并筛选“占用”，保留已选仓库；库位页支持清空仓库查询全部，恢复 URL 中的合法状态（含禁用状态 0）。仓库查询选项包含停用仓库，供历史数据核对；批量生成仍只提供启用仓库，停用仓库不自动带入生成表单。

前端 lint、24 文件 / 249 单测、vue-tsc / build 通过；14 项业务布局和最终 9 项仪表盘 UI fixture 回归通过，覆盖日夜主题、全部/指定仓库跳转、清空条件、停用仓库查看及生成选项。最初新增测试的 Element Plus 占位文本与点击目标定位已修正，最终仪表盘回归无重试。UI 使用拦截接口数据，没有运行真实后端业务链；未改后端，未重跑 Go / MySQL 集成测试。

用户要求仅保存本地 Git 提交，本轮不推送、不触发云端 CI 或部署，线上仍为此前已验收版本。原有外部单功能、迁移和其他未提交文件保留，不纳入本次提交。

## 2026-10-20 运营仪表盘与局部滚动验收

本轮统一修复演示悬浮条遮挡分页、库存查询按钮间距和长内容导致外层页面滚动；悬浮条支持手柄拖动/键盘移动/位置恢复，自动与分步结果按视口分配高度并在内容区滚动。业务列表、单据详情、证据和工程弹窗采用对应局部滚动。随后完成日夜运营仪表盘，详情见 [UI 记录](ui-review.md) 和 [API 汇总契约](api.md)。

仪表盘新增只读权限/仓库汇总，不更改库存写入、状态机、领取、幂等、数据库结构或现有演示运行器。新建单趋势与当前作业状态分别统计，不混加商品单位，不推导没有数据依据的 SLA、准确率或预测值。指标点击携带仓库、创建日期与状态；任务列表增加同范围只读过滤。

独立发布源码快照验证 Go vet、46 包 / 438 个含子测试 pass / 0 跳过测试、lint 0 issues、Go build；真实依赖仅使用本地专用 MySQL23306/Redis26379。新增 dashboard 用例覆盖普通/精确租户 0、仓库范围、软删、日期边界、权限拒绝时不查询对应表以及输出不泄漏凭证；任务专项验证列表与 total 同时按仓库和租户过滤。前端 lint、249 单测、类型/构建通过；33 项 UI fixture 回归涵盖原布局、日夜/窄屏、拖拽/缩屏、局部滚动、分页避让、数据钻取和失败刷新。真实业务 E2E、race、容器和部署结果见下方 GitHub 验收。

保留任务开始前的外部单接口及其他未提交文件；原差异 SHA256 仍为 c554d1987d7f296f64e3f4bf16454929f2bc710c41f1d60f137ce7a27dabf2ed。专用测试实例不连接生产数据，原有未发布迁移 000013 不包含在本次提交。


[文档导航](README.md) · [项目首页](../README.md)

以下命令均从仓库根目录执行，另有说明的除外。下方按日期保留历史验证记录，旧阶段的“未提交”“幂等尚待实施”只代表当时状态；2026-10-18 基础验收及 2026-10-21 D11/D12/D13 后续加固均已完成，当前契约见 [需求与边界](requirements.md) 和 [Go 约定](go-style.md)。

### GitHub 与上线验收

代码提交 `384828b4a6b331ff284bd285c7c391f90a521c6c` 的 [CI](https://github.com/mengw-dev/go-wms/actions/runs/39451136464) 全部成功：前后端检查、Go race/真实 MySQL/Redis、漏洞扫描、容器就绪和最终门禁通过。69 项 E2E 最终通过，其中 68 项直接通过，演示操作记录核对用例重试一次通过；额外 5 次一键演示均零重试成功。保留操作记录核对的偶发稳定性边界，未将其标为已修复。

[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39452653979) 全部成功，服务器日志确认前后端版本均为上述代码 SHA，数据库就绪校验通过。没有连接生产数据库做人工修改。原有未提交差异哈希保持一致；临时测试依赖和预览服务均已停止。

## CI 按改动范围执行（2026-10-20）

每次推送/PR 先运行轻量 `Change scope` 与 CI 策略回归；按整个推送或 PR 的 Git 差异分流，不只判断最后一条提交。删除和重命名前路径也参与识别，比较基线不存在或路径未知时执行完整检查。

| 改动范围 | Go 静态 / 漏洞 / race / 集成测试 | 前端 lint / 单测 / 类型构建 | 容器就绪与完整业务 E2E | 自动部署 |
| --- | --- | --- | --- | --- |
| 仅 `web/**` | 跳过 | 执行 | 执行 | 全部所需门禁通过后执行 |
| 仅 Go / `cmd` / `internal` / `migrations` | 执行 | 跳过 | 执行 | 全部所需门禁通过后执行 |
| 仅 `docs/**`、根 Markdown、LICENSE | 跳过 | 跳过 | 跳过 | 跳过服务器部署，文档站发布保持独立 |
| CI / 部署 / 共享脚本 / 未知路径 / 缺失基线 | 执行 | 执行 | 执行 | 全部所需门禁通过后执行 |

前端 E2E 仍使用临时 MySQL/Redis 跑真实入出库、幂等恢复和引导流程，这不同于 Go MySQL 集成测试。容器检查与发布继续构建同一 SHA 的前后端镜像并核对就绪/版本，不混用旧后端镜像；本轮没有削减库存一致性测试和五次零重试演示回归。

`CI result` 无论上游成功、失败或跳过都会汇总，只允许“计划不需要的任务”跳过，失败/取消/输出缺失或意外跳过均不通过。容器/E2E 的条件显式处理无关任务的 skipped 状态，防止纯前端时被依赖传播跳过。Deploy 只接受通过最终汇总、容器和 E2E 的业务提交；纯文档 CI 成功不会上线，手动 `workflow_dispatch` 保留原有人工发布入口。

同分支同事件的新提交取消过时 CI，部署仍保持不取消的串行锁。没有在路径过滤处跳过整个工作流，因此文档提交仍会产生明确的最终检查结果。

本地验证：`node --test scripts/ci/policy.test.mjs`（4 组策略与真实 Git CLI 回归，覆盖范围分流、失败/取消/误跳过门禁、多提交、重命名与缺失基线），`actionlint v1.7.12` 检查 CI/CD YAML 通过。实际前端提交 `50bdb47` 被识别为 backend=false / frontend=true，历史文档提交 `240738a` 被识别为两者 false。

中文提交 `b74457f` 的[完整 CI](https://github.com/mengw-dev/go-wms/actions/runs/39422953309) 成功：Change scope 自测、前后端检查、容器、53 项 E2E 与额外 5 次零重试演示均通过，本轮没有 flaky；`CI result` 正确汇总通过。工作流与脚本本身改动按共享范围完整检查，后续前端或文档提交按分流执行。[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39424328407) 成功，新增发布资格门禁正确放行业务提交；服务器核对前后端版本 `b74457fbce5af572e29a3669d5f4d709ebd7fa43` 与数据库就绪均通过。本段验收记录仅修改文档，不带 `[skip ci]`，用于云端核对四项重任务和生产部署按规则跳过。

## 2026-10-20 测试装配与隔离 D14 / D16

基线 `4ca487b`。MySQL 临时 schema 入口集中安装实际租户回调，移除各 fixture 重复注册；tenant 测试改为外部测试包以继续通过统一入口验证。新增 `OpenMigratedMySQL`，应用契约在开发 AutoMigrate 与正式嵌入迁移下分别使用真实 `InitDB + app.New`，执行入库/收货上架、出库分配/拣货发货及盘点业务链。核对全部列集合、关键类型、九个租户唯一键与六个数量 CHECK，验证 Create 填租户、Query/Row 隔离及跨租户 Update/Delete 无写入；既有精确租户 0 与批量创建回归继续通过。历史普通字段的空值/默认值及非唯一索引名称差异不在逐字比对保证中。

Redis 按 fixture 自动添加 UUID 前缀，支持现有普通命令、Lua 显式 KEYS、普通与事务流水线；不认识的命令直接拒绝。清理仅扫描删除当前 UUID 前缀；Demo 预约不再预删固定键，五个会话/执行锁测试启用并行。真实 Redis 回归验证同键隔离、EVAL/EVALSHA、流水线隔离、271 个无 TTL 键清理后另一个 fixture 的会话仍在，以及 FLUSHDB/FLUSHALL/SELECT/无 KEYS Lua/SCRIPT FLUSH 被拒绝；受控脚本和单节点边界见 Go 约定。

仅叠加本轮测试基础设施与测试改动的独立发布快照：Go build/vet、强制 MySQL23306/Redis26379 的全量测试通过（45 包、435 个含子测试 pass，失败/跳过用例 0），全仓 lint 最终 0 issues。之后仅修正新测试的错误格式/参数顺序并启用 Redis 用例并行，应用契约专项与全仓 lint 再过；Redis/Demo/单号专项三个包先重复三轮，再由两个独立 Go 进程共享同一 Redis 各重复五轮（parallel=8）全部通过。补充流水线实际写入断言后专项与全仓 lint 再过。原有未提交 123 增/4 删及迁移 000013 未纳入快照，剩余 diff SHA256 与任务前一致。临时 schema 和 Redis 命名空间清理完成，专用实例已关闭。没有修改业务实现、正式迁移、依赖版本或前端；本地不重跑 race、浏览器及真实容器，远端 CI 结果另补。

中文提交 `090f3b18c9e042afea29e49e415641a2729f3dc8` 已推送 main。[CI](https://github.com/mengw-dev/go-wms/actions/runs/39392303382) 全部成功，包含全量导入护栏、Backend/race/真实 MySQL/Redis、漏洞扫描、Frontend、Containers 和 45 条浏览器 E2E；额外 5 次一键演示零重试成功，没有 flaky。日志逐项确认新增 development/release 应用契约与 Redis 隔离清理测试通过。[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39393255584) 全部成功，服务器确认前后端版本 `090f3b1` 与数据库就绪通过。D14/D16 验收完成，剩余工程条目 12 项；本次验收补充为纯文档提交。

## 2026-10-20 演示首页一屏布局与双主题

基线 `8757fb8`，将已确认的浅色样板接入实际 Vue 演示首页。压缩标题区，合并重复流程介绍与完整演示入口，以五阶段业务流程、当前阶段证据、演示方式及三项工程验证组成桌面一屏布局；右侧使用主题变量的浅渐变。白天保持紫色，夜间保持翡翠绿与深蓝底，不新增第三套主题。首页控制栏进入工作区底部，其他页面仍使用原悬浮控制栏。

保留架构图、五阶段切换、真实业务页、引导演示、入库/出库自动执行、完整自动/分步执行、业务证据、操作日志、工程验证、导入机制、重置、恢复和退出。演示方式改为首页直接选择，删除重复的方式选择弹窗；执行、幂等与检查点继续调用原逻辑。另修正阶段状态判断：入库证据不会将无对应证据的出库阶段标为已执行。没有修改 Go 业务、迁移、依赖或原有未提交外部单功能。

本地前端 lint、24 文件/249 单测、包含类型检查的 build 通过。新增 4 条 Playwright 布局与事件契约测试通过：覆盖日夜主题、五阶段、长单号、1920×914/1536×730/1366×650/1280×620/1024×600 无滚动与按钮溢出、320px 无横向溢出、主题切换、完整/局部演示事件、证据与日志路由、重置及退出。该专项使用隔离 API fixture，不代表真实业务链已在本地重跑；既有真实后端 E2E 只更新入口选择，保留业务数量、流水与丢响应恢复断言，远端 CI 结果另行记录。

中文提交 `9c0f28d548c7a8a6f8d04783b8f2fd4ef709b007` 已推送 main。[CI](https://github.com/mengw-dev/go-wms/actions/runs/39388161877) 全部成功：Backend/race/漏洞扫描、Frontend/249 单测、Containers、45 条浏览器 E2E 与额外 5 次零重试均通过，没有 flaky。[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39389086157) 全部成功，服务器确认前后端版本均为 `9c0f28d`，数据库就绪检查通过。使用说明已更新；原有未提交工作 diff 的 SHA256 与任务前完全一致，专用本地前端预览进程已关闭。本次补充为纯文档验收提交。

## 2026-10-20 依赖护栏 D07

基线 `6eb6392`，承接已验收的演示修复。只增加导入检查、校验接线、工具测试和文档，不改业务实现、事务、迁移、依赖版本；原有未提交外部单功能保持原样。

- Go：`go run ./scripts/check-dependencies` 使用标准库 parser 检查全部 internal Go 导入，不按 diff 或当前构建标签过滤。发布快照扫描 247 文件/1799 导入；测试涵盖真实目录扫描、带别名/空白/原始字符串导入、未启用构建标签、未来模块名、空/缺失/坏源码拒绝及现有装配/跨 model/短事务例外。实际构建 CLI 对临时违规树返回退出码 1；全仓生产树通过。CI 和 verify 先执行同一命令，Go 全量测试也检查真实仓库。
- 前端：ESLint 本地规则规范化导入路径，覆盖 JS/TS/Vue、再导出、动态导入、require 和类型导入；27 个允许/拒绝回归使用真实配置与 Vue/TS parser，实际 CLI 反向依赖样本退出码 1。工具测试放在 eslint-rules，由 Vitest 显式纳入，避免 Node 工具类型进入浏览器应用；套件初始化加载配置/parser，保留默认超时与重试策略。
- 独立源码快照：Go build/vet、全仓 lint（0 issues）、强制真实 MySQL23306/Redis26379 的全量测试通过（45 包、430 个含子测试 pass，0 fail/skip）；最后仅简化布尔谓词，专项回归与全仓 lint 再过。前端 lint、24 文件/249 单测、类型检查和 build 通过。actionlint v1.7.7（未启用外部 shellcheck/pyflakes）、修改脚本的 PowerShell 语法及差异检查通过。

中文提交 `33a1892af3637fbf5b89265964c9d4736fc12b5b` 已推送。[CI](https://github.com/mengw-dev/go-wms/actions/runs/39368158887) 全部 success，新增全量导入检查实际通过，Backend/race/漏洞扫描、Frontend/249 单测、Containers、41 条 E2E 与额外 5 次零重试均成功，没有 flaky。[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39368910580) 全部成功，服务器日志确认前后端均为 `33a1892`，数据库就绪通过；D07 验收完成。本地专用 MySQL/Redis 已关闭，原有未提交 123 增/4 删及迁移/脚本等保留；本次验收补充为纯文档提交。没有连接生产数据库；导入护栏仅约束静态直接依赖，不代替业务事务、SQL 与租户审查，详细范围见 [Go 约定](go-style.md)。

## 2026-10-20 演示数据与流程 D15 / D17 / D18 / D19

本轮从 `d011454` 的独立发布源码快照验证，只叠加演示数据/输入解析/运行器及对应测试；保留工作区原有外部单功能和迁移 000013，未混入发布。没有连接生产数据库或自动修改历史业务数据。

- D15：新种子只写基础资料与明确的 OPENING 期初库存，不再造完成态主单；原 system-seed 单据在列表/详情标记来源。真实 MySQL 回归验证期初 4 行/230 数量、流水余额一致、无伪造明细/任务/分配/主单、重复执行与已有数据保留；原 Reset 双租户隔离测试继续通过。
- D17：所有可选场景请求体统一接受空输入或一个 JSON 对象。测试用 nil service 作为副作用探针，断言各入口在 ContentLength=-1/0/已知长度下，非法/截断/null/数组/标量/尾随内容均返回 400，不进入默认操作；类型、绑定范围与默认兼容另有回归。
- D18：旧 full 一次调用后端编排并保留失败部分结果；删除无调用方的拼接工具，前端回归断言不会分段请求及重置。
- D19：同一 sessionStorage 检查点原子保存进度、写目标/参数/操作键及确认状态。前端回归覆盖收货/上架/多任务拣货提交后丢响应、终态原 key 回放、非幂等写只读核对/未命中不重发/重复匹配拒绝、建库位恢复、存储失败/损坏、七天过期、双击、暂停/销毁/切换账号及迟到查询/响应。浏览器用例转发真实请求并等后端成功后丢弃响应，再刷新恢复，核对单据数量和 SHIP 流水；本提交 CI 已实际执行通过。

本地校验：gofmt、Go build/vet、强制独立 MySQL23306/Redis26379 的全量测试通过（44 包，411 个含子测试的 pass 终态，0 fail/skip）；前端 lint、23 文件/222 单测、类型检查与 build 通过。最后的直接返回简化另经 bootstrap 真实数据库回归和全仓 Go lint（0 issues）通过，存储键唯一性与多任务终态恢复另经完整前端检查通过。本地测试服务已关闭；本地未重新启动 API/Docker，真实浏览器、race 和容器由 CI 隔离环境验证。恢复需原会话仍有效，暂停不回滚服务端提交；清存储、跨标签页/设备与七天窗口边界见 [演示说明](demo.md)。

中文提交 `b55dd17eb76ecd4d9f5eed6aadf7a6c1f5471263` 已推送 main。[CI](https://github.com/mengw-dev/go-wms/actions/runs/39365641685) 全部成功，Backend（含 race 与漏洞扫描）、Frontend、Containers、E2E 均通过；E2E 日志确认 `41 passed (3.0m)` 与 `5 passed (1.1m)`，含新增丢响应刷新恢复场景，没有 flaky。[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39366392859) 全部成功，日志确认前后端同为 `b55dd17`，数据库就绪通过。四项修复验收完成；原有未提交业务增删行仍为 123 增/4 删，完整保留。此验收补充是纯文档提交，不重复触发代码流水线。

## 2026-10-20 验证期间的 Excelize 漏洞升级

演示 E2E 修复提交 `9aa0e32` 的 [CI](https://github.com/mengw-dev/go-wms/actions/runs/39361066864) 在漏洞扫描阶段被阻断，未进入 E2E：Go 漏洞库新增的 GO-2026-6684/6685/6688/6689 命中项目实际调用链，门禁正常拒绝。本轮保留扫描策略并升级依赖，没有忽略报告或放宽失败条件。

- Excelize 从 v2.11.0 固定到 `v2.11.1-0.20260930021559-01a9ff32fb3c`，同时按其依赖要求更新 mscfb 到 v1.0.8、efp 到 v0.0.2；主项目 Go 版本保持 1.26.9。选择上游已修复提交，原因是核对时最新正式标签仍为 v2.11.0。修复依据见 [Go 漏洞库](https://vuln.go.dev/ID/GO-2026-6685.json) 与 [上游提交](https://github.com/qax-os/excelize/commit/01a9ff32fb3c1f873cf01205e1b8a3285b0e1d23)。
- 新增 `TestImportRejectsOutOfRangeWorksheetRow`，从正常模板生成小体积 XLSX 后注入超过 Excel 行上限的后续行，直接验证实际 doImport 在读取阶段返回失败、未产生任何业务行处理、未进入 panic 恢复。测试不依赖数据库；正常模板与导入执行仍由既有真实数据库回归覆盖。
- 升级仅叠加到独立发布源码快照：Go build/vet、强制真实 MySQL/Redis 的全量测试（跳过 0 项）及全仓 lint（0 issues）均通过；govulncheck v1.9.0 实际扫描和现有门禁通过，已调用可修复漏洞 0、无修复版本告警 0。测试仅用本地 23306/26379，结束后关闭服务。后续 [CI](https://github.com/mengw-dev/go-wms/actions/runs/39361820930) 全部成功，含 Backend/race、Frontend、40 条 E2E、演示用例额外 5 次无重试及真实容器验证；[自动部署](https://github.com/mengw-dev/go-wms/actions/runs/39362626384) 也成功，日志确认前后端版本 `6e3b384d1ffa1f06b547b97c880c0510660e97ca` 与数据库就绪一致。安全升级与验收已完成；首个失败运行仍保留其真实结果。

## 2026-10-20 一键演示 E2E 稳定性

基线 `main` / `cdce9a5`。上一轮 [CI](https://github.com/mengw-dev/go-wms/actions/runs/39193419819) 的一键演示用例出现 1 flaky：全局 `getByText('创建入库单').first()` 命中了关闭后保留的运行器隐藏节点，并非业务操作记录中的单元格。记录面板每页 5 行、按时间倒序，创建操作也不应假定出现在首页。

状态：已完成。修复提交 `9aa0e32`，安全依赖补充提交 `6e3b384` 均已推送；最终 CI 日志为 `40 passed (2.5m)` 与 `5 passed (1.1m)`，无 flaky，自动部署成功。重复运行是本次覆盖范围的稳定性证据，不保证以后所有环境都不会出现偶发失败。

- 用例将断言限定到“业务操作记录”tabpanel，先确认首页的真实拣货操作，再切换末页。创建记录同时核对本次 POST 建单响应的单号，避免同名步骤或其他单据造成假通过；显式确认关闭运行器中的同名节点仍保留且隐藏，覆盖原故障条件。
- CI 在完整 Playwright 测试后额外重复该用例 5 次，强制 `--retries=0`，任一次失败均阻断 CI。重复测试使用独立结果子目录及 line reporter，保留完整测试的 HTML 和失败附件；不增加等待时长、全局重试次数或固定休眠。
- 发布源码从 HEAD 导出独立快照，仅叠加本次 E2E 修改；本地前端 lint、21 文件/199 单测、类型检查与 build 通过，工作流经 actionlint v1.7.7 检查（未启用外部 shellcheck/pyflakes）。真实业务与重复运行结果由该提交的 CI 提供。
- 本地真实后端启动被自动审批以 `blocked by policy` 拦截，未给出具体原因；本地未取得有效的 E2E 验证结果，真实场景改由 CI 隔离栈验证。已单独核对重复命令确实发现 5 个目标测试。尝试准备的本地专用测试 schema 已删除、MySQL23306/Redis26379 已关闭，未连接线上库。原有未提交业务功能和迁移保留，不纳入本次发布。

## 2026-10-19 部署迁移失败与服务保留

基线 `main` / `9ac0cdd`。CI #164 四个作业全部成功，但 [Deploy #56](https://github.com/mengw-dev/go-wms/actions/runs/39191431517) 的服务器迁移预检失败；原脚本过早重建 API/Web，且提前退出时未采集迁移日志。不能把 CI 成功等同于上线成功。

- 只读诊断确认：生产迁移版本 11、dirty=0；异常是 8 条旧版 system-seed 演示出库主单，数量 30/0/30，与旧种子单号、业务号、备注、版本和仓库逐项一致，没有关联明细、分配、拣货任务或库存流水。
- 用户明确授权线上处理后，先在服务器 `/var/backups/gowms/ci-repair-20261019-*` 留存完整数据库逻辑备份、校验和、修复 SQL 和结果。事务仅匹配审计确认的 8 个主键/租户，检查迁移状态、完整样本特征、无业务关联及异常总量，任何条件不符均失败回滚；实际仅将这 8 条演示样本的 allocated_qty 从 0 修正为 30，库存和业务状态未变。此定点维护未加入自动迁移，不能用于自动修账；D15 完整演示业务链仍待实现。
- `deploy-services.sh` 由服务器发布与 CI 真实容器回归共用：基础服务健康 → migrate 退出成功 → bootstrap-admin 退出成功 → API/Web/Caddy 切换。已有 MySQL/Redis 不重建，失败的一次性容器不再提前替换应用；应用切换后失败仍不自动回滚。
- 发布脚本统一退出诊断覆盖拉取、镜像校验、迁移、初始化、应用启动和最终探针，打印阶段、原始失败码、已退出容器状态及有限日志；诊断失败保留原始退出码。

| 本地验证 | 结果 |
| --- | --- |
| `bash scripts/tests/deploy.tests.sh` | 25 次调用通过；真实临时 Git A/B 远端与 jq，Docker/HTTP 替身。新增阶段顺序、迁移/初始化失败保留应用、错误日志、诊断失败不覆盖原退出码及应用启动失败。 |
| 定点修复 SQL，独立 MySQL 23306 的专用临时 schema | 7 场景通过：准确修复 8 条、重复执行拒绝、额外异常、样本身份不符、存在业务明细、软删、dirty 版本；拒绝场景确认未发生部分更新。测试 schema 与测试服务已清理/关闭。 |
| Shell 语法、diff 检查、actionlint v1.7.7 | 通过；actionlint 未启用外部 shellcheck/pyflakes。 |

CI 的真实容器回归新增迁移与初始化失败注入：比较 MySQL/Redis/API/Web 的容器 ID、启动时间和运行状态，验证旧 API 仍就绪，再恢复成功发布。该回归与全量后端/race、前端、E2E 由本次提交的远端 CI 执行；本地 Docker daemon 未运行，未冒充真实容器测试已在本地执行。未修改 Go/Vue 业务代码、正式迁移或既有未提交外部单功能。

## 2026-10-19 数据库防线 D01 / D23 / D24

基线 `main` / `3a44179`。本轮只修复开发约束建立、数量进度 CHECK 及事务冲突响应；问题总表 D01/D23/D24 已标记完成。没有修改库存锁顺序、业务事务范围或自动修正历史业务数据。

- bootstrap/checks.go 只读审计全部租户/软删行，核对真实 CHECK 表、表达式和 ENFORCED；开发迁移失败不再只告警，release 入口只读核对六个必需约束。
- 新增 000012，为 task/allocation/shipment_order 建立非负与进度上限约束；migrate up 在业务 DDL 与设置 dirty 前先审计。新建演示种子补齐分配量，但 D15 完整明细/任务/流水问题不关闭。
- Tx/TxRetry 对 1205/1213 返回保留驱动 cause 的 50302，日志保留原始错误。1205 不自动重试；1213 在 2 秒后续事务发起窗口内默认最多 3 次，抖动等待合计不足 300ms，业务冲突耗尽保留原 409。窗口不强制中断已开始事务；单轮时限依赖请求 context 和数据库设置。
- 前端四种写操作与领取的回归确认 50302 保留原 key/原参数，恢复成功后才移除 pending；未扩大全局自动重试。

本次从暂存区导出独立发布源码快照 `87ea81597739b2f240bf314e261c1625a755642c`，不包含原有未提交外部单功能；之后只补文档。实际结果：

| 检查 | 结果 |
| --- | --- |
| `go test ./internal/bootstrap ./cmd/migrate ./internal/pkg/tx ./migrations -count=1 -timeout=120s` | 真实 MySQL 专项通过；重复建表、错定义/未启用、真实 3822/3819、并发正确创建、注入 1142 权限错误；release 只读校验；开发/SQL 定义一致、非法写入拒绝、零/INT 最大值、迁移回退重做均通过 |
| CLI 子进程回归 | 历史异租户/软删的 picked>allocated 行被拒绝，原数据不变、迁移仍为 11 且 dirty=false，未先添加部分业务约束；仅修正测试 fixture 后新版 CLI 可正常升级 |
| 真实锁故障与 HTTP | 两事务交叉锁制造真实 1213；新事务重试可见已提交值且失败轮无残留，连续死锁耗尽为 50302；真实 1205 仅执行一次，之前已写行也回滚。业务冲突原码保留、取消不再开事务、超出发起窗口不再重试均通过 |
| `scripts/windows/verify.ps1` | Go 格式/build、全量真实 MySQL/Redis 零跳过、vet、全仓 lint 0 issues、租户原生查询审计；前端 lint、21 文件/199 单测、类型与 build、diff check 全过 |
| 原有工作保护 | 原有已跟踪改动增删行完全一致。为避免占用同一迁移版本，本地未发布的 000012_external_order_source 两个文件仅顺延为 000013，SHA256 与原文件一致，未纳入提交；发布快照只包含新的 000012 数量约束 |

测试只连接本轮专用 MySQL 23306 / Redis 26379，各 fixture 自建测试 schema；没有连接开发或生产业务库。结束后关闭临时服务。本地未跑 race/E2E/k6/真实 Docker；远端 CI 将执行 race、E2E 与真实容器启动检查，本记录在提交前写入，不预报远端通过。历史不合规数据需要业务核对，不自动回填；升级必须备份/停写，DDL 仍可能部分成功并留下 dirty，处理说明见 deployment.md。

## 2026-10-19 部署一致性 D04 / D05 / D06

基线 `main` / `55826ee`。仅修改发布 workflow、Dockerfile、nginx 就绪入口、release Compose、发布脚本及回归；保留原有业务逻辑、迁移和未提交工作。D04–D06 的根因和修复见问题总表，配置与使用边界见 deployment.md。

发布固定完整提交、两个不可变镜像 digest 及三项构建信息；服务器仓库不干净时停止。最终检查从同一 Web 入口解析数据库/API 就绪响应，并逐项核对前后端元数据。SSH 发送 runner 固定提交的脚本，使用脚本自身 `set -eu`，关闭会逐行插入语句并破坏多行控制结构的 script_stop。

| 检查 | 本地实际结果 |
| --- | --- |
| `bash scripts/tests/deploy.tests.sh` | 19 场景通过；实际临时 Git 远端含 A/B 两个提交，最新 main 为 B 仍部署 A；保留 .env，已跟踪/暂存/未跟踪修改均阻断且保留。Docker/HTTP 为替身，jq 为真实 1.8.1。覆盖 API/DB 故障、SPA、连接中断、多个 JSON、前后端旧版、dev/unknown、业务错误/字段缺失、pull/镜像标签失败及非法输入。 |
| `sh -n scripts/deploy.sh`、`bash -n scripts/tests/*.sh` | 本轮两个 Bash 回归脚本与发布脚本语法通过 |
| actionlint v1.7.7 | 当前 CI/Deploy workflow 均通过（本地未启用外部 shellcheck/pyflakes） |
| CI/Deploy YAML、Compose | js-yaml 解析通过；基础 + release + tls Compose 经 Docker CLI config 检查通过，不需要 daemon |
| `scripts/windows/verify.ps1` | 独立暂存源码快照 `d3bdd646c99db380a4bcfa681300b7043bb81818`：Go build、真实 MySQL/Redis 全量测试零跳过、vet、全仓 lint 0 issues、租户原生查询检查；前端 lint、21 文件/194 单测、类型检查与 build、diff check 全过。之后仅补文档、删除 SSH 的 script_stop 开关及加固回归脚本临时目录清理范围；最终 workflow 已单独经过 actionlint 检查，回归脚本已复跑。 |
| 原有未提交修改保护 | 任务前后剩余 diff 的所有新增/删除行完全一致，未把外部单保护、迁移 000012、Makefile/reset/k6 等混入发布 |

测试仅连接本轮启动的本地 MySQL 23306 / Redis 26379，各 fixture 自建测试 schema，没有连接开发或生产业务库。测试后关闭临时服务。本地未运行 race、E2E、k6、生产部署；Docker daemon 未运行，因此本地没有执行真实容器构建/故障测试。CI 的 containers 作业已加入 `deploy-containers.sh`，使用独立项目与测试卷，核对真实前后端构建元数据，并在停止 MySQL/API 后确认 nginx 存活而 readyz 失败。此记录在提交前写入，远端结果以该提交的 Actions 实际记录为准，不把新增检查入口写成远端已通过。

## 2026-10-19 验证门禁 D02 / D03

基线 `main` / `926ad0e`。本轮只修复本地集成测试和 CI 漏洞扫描的假通过问题；没有修改业务运行逻辑、数据库迁移或前端功能。原有外部单保护、迁移 000012、Makefile/Windows/k6 未提交改动保留。verify.ps1 原有本地状态仅为行尾差异，本轮仅提交新增门禁改动。

- D02：`verify.ps1` 调用新的 `test-go.ps1`，默认要求显式测试 DSN/Redis 地址，临时强制 WMS_TEST_REQUIRED=1；JSON 事件区分无测试文件的包与实际测试跳过。缺少配置、测试失败、跳过用例、没有成功测试包、无效 JSON 均不能通过；成功和失败均恢复环境。显式 AllowSkippedIntegration 只能产生部分检查结果，不显示完整通过。
- D03：`scripts/check-vulns` 以子进程运行真实 govulncheck，检查退出状态和完整 JSON 对象流、协议、符号级源码扫描及 SBOM 目标。扫描失败/取消、空/截断/损坏/无目标报告均拒绝；保留已有漏洞策略、去重输出与原始报告，CI 上传 artifact。报告输出路径为本地 CLI 参数、使用 0600 文件权限，单行 gosec 说明限定于该开发者输出边界，不关闭全局规则。
- 验证完整脚本时发现系统级数量审计被既有租户字符串检查误报；仅补充其跨全部 tenant_id 的意图注释，SQL 与运行逻辑不变。D07 模块依赖护栏仍待实施。

从 index 导出独立源码快照 `tmpgen/gates-20261019/source`（源码树 `d2381cb76afea1fcfeb058501f4e6ea00b5dd193`，之后仅补文档/报告忽略规则）执行：

```powershell
$env:WMS_TEST_DSN='root@tcp(127.0.0.1:23306)/?parseTime=true&timeout=2s'
$env:WMS_TEST_REDIS_ADDR='127.0.0.1:26379'
.\scripts\windows\verify.ps1
.\scripts\tests\verification-gates.tests.ps1
go test ./scripts/check-vulns -count=1
go run ./scripts/check-vulns -scanner <本机 govulncheck 路径> -report vuln.json
```

| 验证 | 结果 |
| --- | --- |
| 完整 verify 入口 | 通过：gofmt、Go build/真实 MySQL 与 Redis 全量测试（跳过 0 项）、vet、golangci-lint 0 issues、租户原生查询检查、前端 lint/21 文件 194 单测/vue-tsc/build、diff 检查 |
| PowerShell 门禁回归与语法检查 | 通过：缺 DSN/Redis、失败退出、实际跳过、空/损坏 JSON、无测试文件包、显式部分模式、环境恢复；无 Pester 依赖，已纳入 CI |
| 扫描器进程与报告回归 | 通过：扫描失败/部分输出后失败、缺失扫描器、取消、空/损坏/协议或级别不符/缺少目标报告；可修复已调用阻断、无修复仅告警、模块/导入未调用不升级、重复 finding 去重 |
| 真实扫描（govulncheck v1.8.0，Go 1.26.9） | 完成且门禁通过：无已调用且已有修复版本的漏洞，无修复版本告警 0；仅为本次扫描结论 |
| 真实扫描失败目标 | 无匹配包时扫描器退出 2，门禁退出 1，未输出成功；保留原始报告 |
| CI YAML | 本地解析通过，新增脚本回归及报告上传步骤结构有效；不代表远端已执行完成 |

仅使用本轮启动的本地测试 MySQL 23306 / Redis 26379，各 Go fixture 自建临时 schema；没有连接开发或生产业务库。测试服务在结束后关闭。未运行本地 race、E2E、k6 或生产部署；本快照没有 .env，完整脚本未执行条件式 Docker Compose 配置检查。本轮前端无功能变更，前端检查由完整入口一并执行。

## 2026-10-18 十项 P2 专项

基线 `main` / `e67b5ea`。修复问题总表 F7/F9/F12/F13/F14/F15/F16/F20/F21/F22，并将当前登记未关闭 P1/P2 均更新为 0；工程债务和未完成全仓审查不随之关闭。实现与回归逐项见 review-findings.md 的 P2 专项表。

主要改动：system 的登录/权限/密码；前端 auth/request 和密码规则；task API 与入出库详情聚合、业务选项；五个业务模型的 27 个数量字段、DTO/累计校验、bootstrap 开发数量审计；AI 聚合查询、导入文件补偿、外部单 SKU 身份核对及库存不足口径。新增或扩展 `p2_auth_test.go`、`task_pagination_test.go`、`order_quantity_test.go`、`external_identity_test.go`、`low_stock_test.go`、`import_completed_cleanup_test.go`、`availability_test.go`、bootstrap/migrations 的 `quantity_test.go`、实盘 HTTP 校验，以及前端 request/options/quantity 回归和 P2 选项 E2E。

本轮仅提取 P2 改动进入 index；原有外部单来源/查询/取消/删除保护、迁移 000012、Makefile 和 Windows/k6 脚本保留本地。四个共享出库文件按原工作区与 HEAD 对齐提取增量；不能把工作区整体通过当成拟发布提交通过。实际从 index 导出的独立源码快照（源码树 `5deab6160e8f8befdc51fcf8b7c158960017751b`，后续仅补文档）验证：

```powershell
$env:WMS_TEST_REQUIRED='1'
$env:WMS_TEST_DSN='root@tcp(127.0.0.1:23306)/?parseTime=true&timeout=2s'
$env:WMS_TEST_REDIS_ADDR='127.0.0.1:26379'
go test ./... -count=1
go vet ./...
go build ./...
golangci-lint run ./...
# web/
npm run lint
npm run test
npm run build
$env:E2E_BASE_URL='http://127.0.0.1:25173'
npx playwright test p2-options.spec.ts p1-order-safety.spec.ts idempotency-recovery.spec.ts inventory-lifecycle.spec.ts --reporter=list
```

Go 全量真实 MySQL/Redis 测试、vet/build 和全仓 lint（0 issues）通过；gofmt/goimports 与 diff 空白检查通过。前端 lint、21 文件/194 单测、vue-tsc/build 通过；真实后端 Playwright 12 场景通过，包含新增超过 100 条选项后选择仓库/SKU并真实建单、P1 四场景、幂等恢复三场景及库存闭环四场景。测试依赖为独立本地 MySQL 23306/Redis 26379；E2E 使用专用 `gowms_p2_e2e_20261018` schema、API 28080 与 Vite 25173，没有连接开发或生产业务库。

数量选择保持正式 SQL 的 INT 契约，本轮不改旧迁移、不新增生产收窄迁移、不裁剪历史数据；开发 AutoMigrate 必须在停止其他写入后运行，审计超限值会阻止改表。限流/权限缓存仍是进程内能力，分页聚合不是严格一致读快照。未运行本地 race（CGO_ENABLED=0）、k6、生产部署或全部其它 E2E；远端 CI 状态另行核对，不能以本地通过代替远端结果。

## 2026-10-18 七项 P1 专项

基线 `main` / `f0d7bca`，范围为问题总表 F1/F2/F4/F5/F11/F17/F18。对应实现与回归已补齐；提交说明与 GitHub 推送说明使用中文。当前工作区原有外部集成来源/查询/取消/删除保护、迁移 000012、Makefile 和 Windows/k6 脚本改动保留，不纳入本轮发布；共享的 `outbound/service/order.go` 只提取 P1 主数据锁改动进入暂存区。

### 实现与回归范围

| 问题 | 实现 | 回归 |
| --- | --- | --- |
| F11 | RecordActualReq.ActualQty 为 required/min=0 指针，HTTP 绑定通过后才解引用 | `stocktake/handler/actual_test.go`：真实 HTTP + MySQL，缺失/null/负数不改明细且审核拒绝，显式 0/正数成功 |
| F18 | 正 INT32 坐标，先除法检查总量≤1000，再按数量迭代，前端同步端点上限 | `basic/dto/dto_test.go`、`basic/service/location_limits_test.go`：1000/1001、零/负/反向、极大维度、MaxInt 和最大合法终点，幂等重放 |
| F4 | 清空旧表单、加载代次与 ready/fetching 状态、禁止加载未完成保存、保存中防双击 | `useInboundOrderForm.spec.ts` 四个异步场景与 `p1-order-safety.spec.ts` 的真实后端编辑场景 |
| F5 | Layout 按 route.path 重建 router-view，单据 ID 变化重新初始化 | `p1-order-safety.spec.ts` 三个真实 SPA Router 跳转场景，入库/出库提交、盘点录入/审核均只作用 B，A 不变 |
| F1/F2 | 调用方事务内校验仓库、共享锁批量读取 SKU 快照；仓库→SKU 升序共享锁与删除排他锁互斥，允许并发建单 | `internal/app/order_references_test.go`：单连接池编辑/失败回滚、入出库创建与 SKU 删除两种交错、租户隔离；原外部同号并发回放继续回归 |
| F17 | 新上架写库存前按仓库→库位锁顺序核对当前库位可用性/归属；已成功 key 继续提前回放 | `inbound/service/putaway_location_race_test.go`：禁用先提交拒绝无副作用、上架先持锁则禁用最后生效、禁用后原 key 回放成功 |

仅连接本地专用 MySQL 127.0.0.1:23306 与 Memurai 127.0.0.1:26379，后端集成测试由 OpenIsolatedMySQL 创建回收独立 schema。浏览器用独立测试库；没有连接开发库或生产库。发布前已从暂存区导出源码快照，排除其他本地功能后执行全量校验；结果见本节下表。

### 发布前校验结果

以下检查针对从 Git 暂存区导出的独立源码快照 `tmpgen/p1-release-20261018/source`，不包含外部单保护等其他本地改动。

| 命令/场景 | 结果 |
| --- | --- |
| Go 新增/修改文件 gofmt/goimports；`git diff --cached --check` | 通过 |
| `WMS_TEST_REQUIRED=1 WMS_TEST_DSN=... WMS_TEST_REDIS_ADDR=127.0.0.1:26379 go test ./... -count=1` | 全量通过，真实 MySQL/Redis；含本轮新增 HTTP、单连接池、主数据并发和回滚回归 |
| `go vet ./...`、`go build ./...` | 通过 |
| `golangci-lint run ./...` | 通过，0 issues |
| web：`npm run lint`、`npm run test -- --reporter=dot`、`npm run build` | 通过；18 文件、177 单测；build 含 vue-tsc |
| web：`E2E_BASE_URL=http://127.0.0.1:25173 npm run test:e2e -- p1-order-safety.spec.ts idempotency-recovery.spec.ts inventory-lifecycle.spec.ts --reporter=line` | 11 passed；4 个 P1 编辑/真实 SPA 路由回归、3 个幂等恢复、4 个原业务闭环 |

E2E 使用独立 `gowms_p1_release_e2e_20261018` schema，前后端均由这份提交快照启动（API 28080、Vite 25173）。未运行本地 race（CGO_ENABLED=0）、k6 或生产部署；数据库数量字段类型、其他 P2 与工程债仍按原范围保留。

## 2026-10-18 幂等专项：已完成并通过本地验收

基线：`main` / `ca24141`；`git ls-remote origin refs/heads/main` 已核对 GitHub main 同一提交。依据 `docs/review-findings.md` 的 F3/F6/F10，含相关 F8 与 F1 领取分支，本次提交包含本轮幂等实现、测试及文档。既有外部单保护、迁移 000012、Makefile/Windows 脚本/k6 修改完整保留，不属于本轮新增实现。

### 修复及文件范围

- 后端：`internal/modules/inbound/service/{receiving,putaway}.go`、`internal/modules/stocktake/service/approve.go`、`internal/modules/outbound/service/pick.go`、`internal/pkg/errcode/errcode.go`。五条 reconcile 读取失败/取消返回 50301 / HTTP 503，不能退回确定拒绝促使客户端丢 key；领取冲突诊断复用当前事务的锁读，避免再借连接。没有变更库存计算、业务指纹或迁移。
- 前端：`web/src/composables/inbound/{useReceive,usePutaway}.ts`、`outbound/usePdaPick.ts`、`stocktake/useStocktakeApprove.ts`；恢复复用普通提交校验，发送中不能放弃，提交前检查存储读取，状态按账号/演示会话隔离，旧身份晚返回保留原 pending；storage 监听随组件/effect scope 销毁注销。
- 入口：`web/src/components/{ReceiveDialog,PutawayDialog,PickDialog}.vue` 与三个业务详情页、盘点列表。修正 storageError 嵌套 Ref；终态恢复入口独立于新作业入口，上架/拣货可选择有 pending 的已完成任务并优先选中待确认项。
- 新增回归：`internal/app/{claim_pool,reconcile_failure}_test.go`、入库/盘点 `service/reconcile_failure_test.go`、`web/src/composables/operationRecovery.spec.ts`、`web/tests/e2e/idempotency-recovery.spec.ts`。已有 `internal/app/idempotency_race_test.go` 的屏障改在 SELECT 完成且未命中后通知，Insert 失败回滚用例改为拣满请求以实际经过发货库存/流水分支。

### 实际环境与结果

使用独立 MySQL 8.0.46（127.0.0.1:23306）、Memurai（127.0.0.1:26379），MySQL data dir 为 `tmpgen/idem-20261018/mysql`，没有连接开发库或生产库。后端测试由 OpenIsolatedMySQL 创建/回收临时 schema；浏览器用独立 `gowms_idem_e2e_20261018` schema，当前源码编译的后端在 28080、Vite 在 25173。未使用业务响应 mock 证明并发或库存正确性。

| 命令 | 结果 |
| --- | --- |
| gofmt/goimports（本轮 Go 文件） | 通过 |
| `WMS_TEST_REQUIRED=1 WMS_TEST_DSN=... WMS_TEST_REDIS_ADDR=127.0.0.1:26379 go test ./... -count=1` | 通过；真实 MySQL/Redis；含五命令核对失败故障注入、领取单连接池、并发与回滚回归 |
| `go vet ./...` / `go build ./...` | 通过 |
| `golangci-lint run ./...` | 1 条既有问题：本轮开始前已存在的未跟踪 `internal/pkg/errcode/integration.go:3` 导出变量注释不以 ExternalOrderProtected 开头（ST1022）；未改该外部单保护文件 |
| `npm run lint` / `npm run test -- --reporter=dot` / `npm run build`（web） | 通过；17 文件、173 单测；build 含 vue-tsc |
| `E2E_BASE_URL=http://127.0.0.1:25173 npm run test:e2e -- idempotency-recovery.spec.ts inventory-lifecycle.spec.ts --reporter=line`（web） | 7 passed；3 个新恢复场景覆盖收货/上架/领取/拣货/盘点审核，另 4 个原有入库导入/入库/出库/盘点闭环回归 |
| `git diff --check` | 通过 |

E2E 故障注入先通过 route.fetch 把请求发送到真实后端，确认响应 code=0 后丢弃响应，刷新页面，从终态/任务入口恢复并断言 key 与原业务参数相同，库存/任务/单据数量不重复累加。拣货的 claim_token 不属于业务指纹，刷新后恢复无须重建旧凭证。

发布前再次从 Git 暂存区导出独立源码快照（`tmpgen/idem-release-20261018/source`），不包含其他未提交功能；该快照的 `go test ./... -count=1`（强制真实 MySQL/Redis）、`go vet ./...`、`go build ./...`、`golangci-lint run ./...` 均通过（0 issues），前端 lint、173 单测与 build 通过。上表 ST1022 仅存在于原工作区的另一个未提交文件，不属于本次提交，也没有通过放宽 lint 规则绕过。E2E 沿用本节已完成的 7 场景真实后端验证。

未运行 Linux race（当前 CGO_ENABLED=0、Docker daemon 未启动）、k6、全套其它 E2E 或生产部署。保留 7 天同 key 窗口；不承诺跨 key、跨标签页互斥、清缓存/跨设备后的自动恢复。F1 入库编辑、F2/F4/F5/F7/F9 等其他专项未处理。

## 本地启动

### 1. 环境要求

- Go 1.26+
- Node.js 20.19+ 或 22.12+
- MySQL 8.0.16+
- Redis 7（缓存和单号可降级，演示会话需要 Redis）

### 2. 启动后端

```bash
mysql -uroot -p -e "CREATE DATABASE IF NOT EXISTS gowms DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
go run ./cmd/wms
```

> 数据库名沿用历史名称 `gowms`（仅内部标识符，与项目显示名 WMS 无关）。Docker 部署可通过 `.env` 的 `MYSQL_DATABASE` 修改；本地开发需同步修改 `configs/config.yaml` 中的 DSN。

服务默认监听 `http://127.0.0.1:8080`。`debug` 模式首次启动会通过 AutoMigrate 建表并初始化管理员；需要验证版本化表结构时先执行迁移；`release` 必须先迁移并初始化管理员：

```bash
go run ./cmd/migrate up              # 只做数据库结构迁移
go run ./cmd/migrate bootstrap-admin # 创建/修复平台管理员（release 必须提供 WMS_ADMIN_PASSWORD）
```

仅在 debug 开发或测试环境需要演示种子时执行：

```bash
go run ./cmd/migrate seed-demo
```

```text
用户名：admin
密码：debug 模式默认 admin123（可用 WMS_ADMIN_PASSWORD 覆盖）；release 模式取 WMS_ADMIN_PASSWORD，未设置会初始化失败
```

首次登录后请立即修改密码。

### 3. 启动前端

```bash
cd web
npm ci
npm run dev
```

访问 `http://127.0.0.1:5173`。开发服务器会将 `/api` 代理到 `http://127.0.0.1:8080`。

### 4. 提交前完整验证

```powershell
# 配置你自己的本地/专用测试实例，以下地址与密码仅作示例；禁止使用生产实例。
$env:WMS_TEST_DSN = 'test_admin:your-test-password@tcp(127.0.0.1:23306)/?parseTime=true&timeout=2s'
$env:WMS_TEST_REDIS_ADDR = '127.0.0.1:26379'
.\scripts\windows\verify.ps1
```

默认通过 `test-go.ps1` 强制 `WMS_TEST_REQUIRED=1`，结束后恢复调用前的环境变量。配置缺失、依赖不可用、Go 失败或测试用例跳过均阻断；Go 的“没有测试文件”包不会误算作测试跳过。只想在缺少依赖时做部分检查，可显式运行 `verify.ps1 -AllowSkippedIntegration`，脚本会报告实际跳过数，并始终标记不能作为完整验证通过。后端单独使用同样门禁可运行 `scripts/windows/test-go.ps1`。

需要同时运行 Linux race 检查时：

```powershell
.\scripts\windows\verify.ps1 -WithRace
```

`verify.ps1` 和 `e2e.ps1` 会检查 Node.js 版本；要求为 20.19+ 或 22.12+。

如果需要额外运行真实后端 Playwright E2E：

```powershell
.\scripts\windows\verify.ps1 -WithE2E
```

### 5. 生成交付压缩包

不要手动压缩整个工作目录，以免把 `.env`、`.git` 或测试产物带入交付包。使用标准脚本：

```powershell
.\scripts\windows\package.ps1
```

脚本要求工作区干净，并使用 `git archive` 只打包当前提交中的跟踪文件；默认输出到 `dist/wms-<commit>.zip`。

## 漏洞扫描门禁

CI 安装 govulncheck 后使用 `go run ./scripts/check-vulns -scanner <扫描器路径> -report vuln.json`。该入口先执行符号级源码扫描，扫描器失败（含加载/网络/取消）、报告为空或 JSON 损坏、协议/扫描级别不符、缺少目标记录均失败。原始报告保留，CI 使用 `go-vulnerability-report` artifact 上传。

策略保持原样：实际调用到且已有修复版本的漏洞阻断；实际调用到但上游未提供修复版本的漏洞告警；仅出现于模块/导入层面的 finding 不升级为实际调用。通过表示本次扫描完成且没有触发上述阻断条件，不等于所有依赖永久无漏洞。CLI 的 `-report` 是开发者/CI 指定的本地输出位置，不接受业务 HTTP 输入。

门禁回归无需 MySQL/Redis：`go test ./scripts/check-vulns -count=1` 和 `scripts/tests/verification-gates.tests.ps1`；后者无需 Pester，验证真实 PowerShell 入口的配置、退出码、跳过与环境恢复规则。CI 同时执行这两类回归。

## 测试与检查

后端：

```bash
go test ./...
WMS_TEST_REQUIRED=1 go test ./... -v -count=1
go vet ./...
CGO_ENABLED=1 go test -race ./... -count=1
```

上面的环境变量赋值语法适用于 Bash。PowerShell 使用 `$env:WMS_TEST_REQUIRED = "1"` 等写法。

MySQL 测试通过 `internal/testutil` 创建和删除独立的 `gowms_test_*` 数据库；用 `WMS_TEST_DSN` 指定本地或专用测试实例，用 `WMS_TEST_REDIS_ADDR` 指定测试 Redis。账号需要创建测试库的权限，禁止指向生产实例。依赖不可用时部分测试默认跳过；`WMS_TEST_REQUIRED=1` 用于将缺失依赖视为失败。

Windows 本机如果没有 GCC，可以使用 `verify.ps1 -WithRace` 的 Linux 容器路径，或由 Ubuntu CI 执行 race 检测。

前端：

```bash
cd web
npm run lint
npm test
npm run build
```

端到端测试（Playwright）：

```powershell
.\scripts\e2e.ps1
```

脚本会启动独立的 Compose 项目和独立数据库，执行登录权限、入库、出库、盘点关键流程，结束后自动删除测试容器和数据卷。首次运行会构建镜像并下载 Chromium，之后可以加 `-NoBuild` 跳过镜像构建。

测试报告生成在 `web/playwright-report`。

### k6 业务流程与压测

场景、前置数据、参数与结果解释统一见 [压测指南](load-testing.md)。

## 维护约定

- 修改前阅读 [代码索引](code-index.md) 和相关测试；Go 约定见 [go-style.md](go-style.md)。
- 前端业务页面只通过 `web/src/events/businessEvents.ts` 与 Guide/Demo 交互。
- 架构展示页以 `web/public/overview.html` 为源文件；修改后在 `web/` 执行 `npm run sync:overview`，构建会检查 `docs/index.html` 是否同步。
- `web/go.mod` 是 Go 工具的扫描边界；`.cmd` 是 Windows 双击入口，实际逻辑在 `.ps1`，两者均保留。

## 本地验证记录

### 2026-10-10：并发快照与回放边界修复（评审反馈 P1×3 / P2×2）

外部评审用定向复现测试指出 3 个 P1、2 个 P2，评委复验后又指出 1 个新的 P1。两轮修复：

- 盘点审核与收货（approve.go / receiving.go）：幂等预查询**整体移出 TxRetry 回调**（独立连接、查完即还），事务内只保留业务锁读写与末尾 Insert。两轮分别消除两类问题：事务内第一条普通 SELECT 提前固定 REPEATABLE READ 读视图导致旧快照；事务占住连接后再向同一连接池申请第二条连接，池满时所有请求互相等待。交错录入实盘按最新实盘调整库存；并发收货收齐判断能看到对方提交、正常生成上架任务。
- 上架（putaway.go）：幂等回放提前到库位校验之前（库位可用性只对新操作校验），事务内不再查幂等（并发同 key 撞唯一键由 TxRetry 重跑 + reconcilePutaway 新读兜底）；完成统计的读视图在订单行锁之后固定，同单并发上架全部完成后单据正常 COMPLETED。
- 外部出库单（integration.go）：SKU 编码统一按主数据规范编码参与去重与内容比较，数据库大小写不敏感命中不再导致同内容重试误报 409。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go build ./...`、`go vet ./...`、`git diff --check` | 通过 |
| `golangci-lint run ./internal/modules/inbound/... ./internal/modules/stocktake/... ./internal/modules/outbound/... ./internal/app/...` | 0 issues（顺带清理上架测试 fixture 的 unparam 告警） |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 全量包 ok，exit 0 |
| 新增回归测试 7 个 | 实际执行：审核与实盘录入交错按最新实盘调整（库存=7 而非 8）；同单不同明细并发收货后单据 PUTAWAY 且生成 2 个上架任务（连接池上限 2）；同单两任务并发上架后单据 COMPLETED（连接池上限 2）；上架成功后禁用库位同 key 重试回放成功、新 key 才被 20011 拒绝；外部出库 SKU 大小写变体回放不 409、变体重复行 50007、快照落规范编码；连接池上限 1 下带 key 收货/审核及其重试完成 |

该阶段未运行：race、Playwright E2E、k6；阶段结束时修改尚未提交，后续状态以版本历史为准。

### 2026-10-10：外部出库单同号内容校验验证（阶段 4）

在盘点审核基线上实现外部 OMS 推送出库单按业务单号的内容校验：`CreateExternal` 先解析并规范化内容（仓库按编码解析、明细按「SKU 编码+数量」集合表示且顺序无关、备注精确比较、重复编码 50007），首次查询命中与并发撞唯一键回查两条路径统一经 `replayExternalOrder` 核对——一致回放原单（`idempotent: true`），不一致 409（业务码 `50012`），历史明细缺编码快照无法可靠核对时报 `50013` 待人工处理、不放行。比较只用创建期不可变字段（`warehouse_id`/`remark`/明细 `sku_code`/`expected_qty`，全仓无更新点），不新增指纹列与迁移；业务单号去重随订单生命周期保留，不依赖 7 天请求级幂等清理。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go build ./...`、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/app/... ./internal/modules/outbound/... ./internal/pkg/errcode/...` | 0 issues |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 全量包 ok，exit 0，无 FAIL |
| 新增 `internal/app/integration_test.go` 用例 | 7 个用例实际执行：同号同内容回放（返回原单不重复建单）、明细顺序不影响相等、数量/SKU 集合/备注/仓库变化均 409（50012）、跨租户同号不同内容独立、并发同号异内容只建一张单且后到者 409、历史订单缺编码快照报 50013；原有 2 个外部集成用例回归通过 |
| 前端 | 本阶段无前端改动，未运行 npm 检查 |
| `git diff --check` | 通过（仅历史存在的 CRLF 提示） |

该阶段未运行：race、Playwright E2E、k6；该阶段结束时与盘点审核成果均尚未提交，后续状态以版本历史为准。

### 2026-10-10：盘点审核请求级幂等验证（阶段 3-3）

在收货/上架基线上新增 `stocktake.approve` 幂等（后端 `approve.go`，scope `stocktake.approve`，指纹=单据 ID——审核无业务请求体；空成功标记 + reconcileApprove），handler 与 demo 调用方同步加参；前端新增 `composables/stocktake/useStocktakeApprove.ts` + 接入列表/详情两个审核入口、`api/stocktake.ts`。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/stocktake/...` | 0 issues |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail |
| 后端新增 `approve_idempotency_test.go` | 4 个用例实际执行：终态后同 key 重放成功（不再误报 60002）且库存/流水/明细不重复调整、未录全拒绝不留记录且补录后同 key 可复用、同 key 用于其他单据 409、损坏标记 500、跨租户与 scope 隔离 |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；新增 `useStocktakeApprove.spec.ts` 10 个用例，全仓 148 项单测通过；构建含类型检查 |

未运行：race、Playwright E2E、k6（与阶段 2 相同的边界）。外部出库单内容校验在阶段 4 落地，验证见上方小节。

### 2026-10-10：上架请求级幂等验证（阶段 3-2）

在收货基线上新增 `inbound.putaway` 幂等：后端 `putaway.go`（scope `inbound.putaway`，指纹=任务/库位/数量，空成功标记 + reconcilePutaway），handler 与 demo 调用方同步加参；前端新增 `composables/inbound/usePutaway.ts` + 改造 `PutawayDialog.vue`、`api/inbound.ts`。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/inbound/...` | 0 issues |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail |
| 后端新增 `putaway_idempotency_test.go` | 3 个用例实际执行：同 key 重放不重复增加库存/流水/任务进度且完成后重放不重复写单据状态、同 key 改数量/库位 409、任务推进注入错误时库存/流水/库位状态/任务进度整体回滚、损坏标记 500、跨租户与 scope 隔离 |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；新增 `usePutaway.spec.ts` 11 个用例，全仓 138 项单测通过；构建含类型检查 |

未运行：race、Playwright E2E、k6（与阶段 2 相同的边界）。盘点审核幂等与外部出库单内容校验仍待实施。

### 2026-10-10：收货请求级幂等验证（阶段 3-1）

在阶段 2 基线上新增 `inbound.receive` 幂等（后端）与前端收货操作生命周期。后端修改 `receiving.go`、`handler.go`、`pkg/idempotency`（空成功标记），调用方（demo/既有测试）同步加参；前端新增 `composables/inbound/useReceive.ts` + 改造 `ReceiveDialog.vue`、`api/inbound.ts`。使用本地 MySQL 8.0 与 Memurai，`WMS_TEST_REQUIRED=1`。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件）、`go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/inbound/... ./internal/pkg/idempotency/...` | 0 issues（修 nilerr 2 处后） |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail |
| 后端新增 `receive_idempotency_test.go` | 4 个用例实际执行：同 key 重放不重复累计（含残品只计一次）、新 key 再收正常累计、收齐后重放不重复生成上架任务、同 key 改数量/明细/批次 409、业务失败不留记录、损坏标记 Internal、租户与 scope 隔离 |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；新增 `useReceive.spec.ts` 11 个用例，全仓 127 项单测通过；构建含类型检查 |

未运行：race、Playwright E2E、k6（与阶段 2 相同的边界）。上架、盘点审核幂等与外部出库单内容校验仍待实施。

### 2026-10-10：幂等并发回放加固验证（阶段 2）

代码基线 `main` / `c8de262`（PDA 前端操作生命周期提交之后）。本次修改 `internal/modules/outbound/service/pick.go` 并新增 `internal/app/idempotency_race_test.go`。使用本地 MySQL 8.0（`MySQL80` 服务）与 Memurai（`127.0.0.1:6379`），仅访问本地测试依赖，未连接生产实例。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l`（改动文件） | 通过，无输出 |
| `go vet ./...` | 通过 |
| `golangci-lint run ./internal/modules/outbound/service/... ./internal/app/...` | 通过，0 issues（修复 nilerr 4 处后） |
| `go test ./... -count=1`（`WMS_TEST_REQUIRED=1`、`WMS_TEST_REDIS_ADDR=127.0.0.1:6379`） | 41 个包全部 ok，0 fail，无跳过 |
| 新增并发幂等测试 | 同 key 并发部分拣货 / 拣满剩余 / 异内容 409 / 并发领取不轮换凭证 / 插入失败整笔回滚 / 损坏快照形状校验 / 租约过期回放 / 租户与 scope 隔离，全部实际执行通过 |
| 原有回归 | 取消↔拣货竞态、PDA 契约、指纹与清理等 `internal/app` 既有测试继续通过 |

未运行：race（本地无 CGO 环境，需 `verify.ps1 -WithRace` 的 Linux 容器）、Playwright E2E、k6。前端本阶段无改动，未重跑前端检查；阶段 1 的前端 lint/test/build 结果以 `c8de262` 提交为基线。

### 2026-10-10：文档更新验证

代码基线 `main` / `4e35b92`；本次只改文档，保留原有脚本改动。使用 Go 1.26.9，仅向本地 `127.0.0.1` 的测试依赖地址发起检查，没有连接生产实例。

| 检查 | 结果 |
| --- | --- |
| `gofmt -l cmd internal migrations`、`go vet ./...` | 通过，格式检查无输出 |
| `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2 run` | 通过，0 issues |
| `go test ./... -count=1 -json` | 退出码 0；按含 Test 字段的终态事件统计（含子测试）：156 pass、96 skip、0 fail |
| 前端 `npm run lint` / `npm run test` / `npm run build` | 通过；12 个测试文件、89 项单测；构建含类型检查与 overview 同步校验 |
| 本轮修改文档的本地链接目标 / `git diff --check` | 通过；93 个本地链接目标均存在，无差异空白错误 |

后端测试显式使用本地测试 DSN、Redis 地址，`WMS_TEST_REQUIRED=0`；本地 MySQL/Redis 不可用导致 96 项跳过，包含库存并发、PDA 后端、租户隔离和迁移相关场景，**本轮不能认定这些集成场景已通过**。JSON 日志保存在执行机临时目录 `wms-maintainability-20261010-tests.jsonl`，不作为仓库持久化产物。本轮未运行 race、E2E、压测、Docker 部署或远端 CI；修改业务逻辑后应在专用依赖可用时设置 `WMS_TEST_REQUIRED=1` 复验。

### 2026-10-07：历史验证

2026-10-07，仓库文档与闲置代码整理后执行：

| 检查 | 结果 |
| --- | --- |
| gofmt、`go build ./...`、`go vet ./...` | 通过 |
| golangci-lint v2.13.2 | 0 issues |
| `go test ./... -count=1 -json` | 263 项通过，6 项 Redis 相关测试因本机依赖不可用跳过 |
| 前端 lint / Vitest / build（含类型检查） | 通过，12 个测试文件、89 项单测 |
| 文档本地链接与 `git diff --check` | 通过 |

此次没有运行 race 或 E2E，也不代表部署环境验收。后续修改需重新执行相应检查；CI 流程见 [ci.yml](../.github/workflows/ci.yml)。
