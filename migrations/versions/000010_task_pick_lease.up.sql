-- 000010: 拣货任务作业租约（PDA 领取）：记录当前持有人与领取凭证。
-- 无租约 / 租约已过期 / 本人持有时可领取或续领；领取凭证用于拣货提交校验，
-- 避免多名拣货员同时作业同一任务，断线后租约到期允许他人接手。

ALTER TABLE wms_task
    ADD COLUMN claimed_by VARCHAR(64) NOT NULL DEFAULT '' AFTER operator,
    ADD COLUMN claim_token VARCHAR(64) NOT NULL DEFAULT '' AFTER claimed_by,
    ADD COLUMN lease_expire_at DATETIME(3) NULL AFTER claim_token;