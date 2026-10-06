-- 000009: 请求级幂等表（收货/上架/拣货/盘点审核等有副作用命令共用）。
-- 唯一键 (tenant_id, scope, idempotency_key) 兜底并发同 key：
-- 后到者插入冲突 → 事务重试 → 命中已有记录，回放首次成功结果。
-- 幂等记录与业务写入同事务提交，业务失败时随事务回滚，key 可复用。
-- result_json 非空：首次成功快照是重试回放的唯一来源，缺失/损坏按内部错误处理，不用当前进度兜底。

CREATE TABLE IF NOT EXISTS wms_idempotency (
  id              BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  tenant_id       BIGINT NOT NULL DEFAULT 0,
  scope           VARCHAR(64) NOT NULL,
  idempotency_key VARCHAR(64) NOT NULL,
  request_hash    VARCHAR(64) NOT NULL,
  object_id       BIGINT NOT NULL DEFAULT 0,
  result_json     TEXT NOT NULL,
  created_at      DATETIME(3),
  UNIQUE KEY uk_idem_tenant_scope_key (tenant_id, scope, idempotency_key)
) ENGINE=InnoDB;