# 单实例备份与恢复

这是基础单实例部署的人工操作流程，不包含定时备份、异地存储、binlog时间点恢复或自动切换。实际经营前应指定备份负责人、频率和保留位置；本地演练结果不能代替服务器恢复验收。

## 备份的内容

一次恢复点应同时保存：

- MySQL完整业务库，包含`schema_migrations`、租户、用户、单据、任务、库存、流水、幂等及移库记录。
- `WMS_UPLOAD_DIR`对应的上传目录，尤其是尚未完成或待重试的Excel任务文件。数据库备份不包含这些文件。
- Redis业务单号计数`gowms:orderno:{RK|CK|PD}:{yyyyMMdd}`及到期时点。停写后保存水位；不把登录会话、演示租约或可重建缓存作为必须回放的业务状态。
- 当前前后端提交/镜像digest、迁移版本、部署配置与密钥。密钥单独受限保管，不提交Git，也不放在可公开下载的备份目录。

Redis不作为库存真相来源，但也不能当成全部可丢弃的缓存：同日把已存在单据的数据库连到空Redis，单号会从1重新发，唯一约束虽防覆盖，建单仍可能失败。恢复演练使用独立Redis并先恢复单号水位，重新登录建立会话；不把旧演示租约或缓存拷回。单号边界继续见[需求说明](requirements.md)。

## 最小人工备份流程

1. 安排停写窗口，停止API和导入worker，确认没有其他实例或外部调用方继续写入。保留MySQL运行，不执行reset或删除数据卷。
2. 记录当前应用版本，以及`SELECT version, dirty FROM schema_migrations`。dirty必须为0；异常状态先查明原因，不使用force掩盖。
3. 使用与服务器兼容的MySQL客户端导出，检查退出码与文件大小，保存SHA-256。
4. 在仍停写时保存Redis单号计数及绝对到期时点，复制上传目录及所需配置/密钥，保持原目录结构和访问权限；备份放到独立受限位置，不能只存于同一业务磁盘。
5. 完成后按原版本启动，检查就绪、登录、单据与导入队列，再恢复业务写入。

下面是PowerShell中使用原生MySQL客户端的示例。`backup-client.cnf`是本机受限文件，按实际环境填写`[client]`下的host、port、user、password；不要使用项目默认密码。路径、库名和工具路径须替换为实际值；有空格的路径使用PowerShell引号。

```powershell
$snapshot = 'D:/private-backups/wms-20261022-1700'
New-Item -ItemType Directory -Path $snapshot -ErrorAction Stop | Out-Null
& 'D:/mysql/bin/mysqldump.exe' --defaults-extra-file=D:/private/backup-client.cnf `
  --single-transaction --quick --hex-blob --no-tablespaces --set-gtid-purged=OFF `
  --routines --events --triggers --default-character-set=utf8mb4 `
  --result-file="$snapshot/database.sql" gowms
if ($LASTEXITCODE -ne 0) { throw '数据库导出失败，不能作为成功备份' }
if ((Get-Item -LiteralPath "$snapshot/database.sql").Length -eq 0) { throw '备份为空' }
Get-FileHash -LiteralPath "$snapshot/database.sql" -Algorithm SHA256
```

不要通过PowerShell文本重定向或`Get-Content | mysql`转码SQL。`--result-file`直接写入客户端原始输出。上述导出不使用`--databases`，便于演练时恢复到新的库名，不会在文件中主动切回原业务库。该流程以InnoDB业务表为前提，停写期间也不做DDL。

## 恢复到全新的隔离环境

1. 验证文件哈希，在独立MySQL实例创建**新库**；保留原库不动。检查SQL头部没有`CREATE DATABASE`/`USE`把目标切回原库。
2. 用MySQL客户端把备份导入新库，遇错即停止，不传`--force`。原生客户端可用`--execute="source D:/private-backups/wms-20261022-1700/database.sql"`读取文件；此示例路径不带空格。
3. 在独立主机/容器文件系统恢复上传目录，配置数据库、Redis与端口均指向演练环境；关闭演示/体验账号，禁止连接原业务系统或外部消息通道。当前导入任务保存文件路径：恢复后的进程内绝对路径须与备份任务记录一致，不能仅修改`WMS_UPLOAD_DIR`就假定旧任务跟随搬迁。若必须改路径，先停worker并人工核对路径迁移方案，不让恢复进程读取原业务磁盘；共享文件边界见问题总表B03。
4. 先使用与备份一致的应用版本，不对恢复库盲目执行最新版迁移。确认迁移版本和dirty，并按下节恢复/核对单号水位；完成前不启动接收写入的服务。
5. 核对表集合、各表精确行数、关键记录和数量约束；同版本停写快照可逐表比较`CHECKSUM TABLE ... EXTENDED`，任意NULL/不支持结果不能视为相等证明。核对库存三数量、任务/单据进度、移库成对流水及待导入文件。
6. 启动release程序，确认数据库约束检查、API健康、浏览器登录及至少一张入库/出库单可读。另用独立测试货品验证写入，不修改真实历史单据。
7. 记录备份时间、文件大小/哈希、恢复耗时、核对结果与问题。演练成功不自动授权切换真实系统；切换需另外安排停写和最终备份，避免丢失演练期间的新业务。

不得通过删除幂等或移库记录来“修复”恢复。保留相同业务历史和原操作键，浏览器待确认操作须与恢复点逐笔核对；恢复到更早的数据点后，不能推定后续已交付的实物操作从未发生。

## Redis单号水位核对

所有发号服务与worker保持停止；先确认目标Redis实例、逻辑库和业务日/服务端时区。用`SCAN MATCH gowms:orderno:*`逐项保存key、整数值及PTTL，并记录备份时刻以计算绝对到期时点，不使用`KEYS *`或全库清空。

恢复每个前缀/日期时，目标值至少取“备份水位、目标Redis现有水位、恢复库已用序号最大值”三者的最大值，不能降低现有计数；保留有效剩余TTL，过期的旧日key不必重建。核对数据库已用号时覆盖全部租户和软删记录，只计标准数字后缀，UUID降级号不参与数值转换。例如实际业务日为20261022时：

```sql
SELECT COALESCE(MAX(CAST(SUBSTRING(order_no, 11) AS UNSIGNED)), 0)
FROM wms_receipt_order WHERE order_no REGEXP '^RK20261022[0-9]+$';
SELECT COALESCE(MAX(CAST(SUBSTRING(order_no, 11) AS UNSIGNED)), 0)
FROM wms_shipment_order WHERE order_no REGEXP '^CK20261022[0-9]+$';
SELECT COALESCE(MAX(CAST(SUBSTRING(order_no, 11) AS UNSIGNED)), 0)
FROM wms_stocktake_order WHERE order_no REGEXP '^PD20261022[0-9]+$';
```

分别对应`gowms:orderno:RK:20261022`、`CK:20261022`、`PD:20261022`。来源水位晚于备份库时可保留更高值，允许跳号，不能为了连续编号往回调。完成后才启动新服务，用独立测试单据验证建单与导入；40009“单号重复”应先查水位，不删除旧单或反复重试直到序号碰巧用完。

本轮停写后仅把三个单号key按不降低水位的方式恢复到独立逻辑库；未恢复会话/缓存，没有执行FLUSH。目标环境仍需按实际账号权限实现受限备份与恢复，不直接照搬本机免密码配置。

## 本轮验证与剩余边界

2026-10-22的隔离本机演练及核对结果见[收尾验收](release-acceptance.md)。本流程未配置定时任务、备份加密/异地存储、保留轮转、PITR或目标服务器RPO/RTO。上线真实业务前，至少确认定期备份能在独立环境恢复；只生成SQL文件不算恢复验收。
