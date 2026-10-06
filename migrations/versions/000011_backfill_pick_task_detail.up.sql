-- 000011: 回填拣货任务的 detail_id（迁移 000003 起拣货任务未写入该字段）。
-- 拣货新增「任务 ↔ 分配行 detail_id 一致性」校验；历史在途任务必须先补上，
-- 否则会因 detail_id=0 被误判为关系不一致而拒绝拣货。幂等：只补 detail_id=0 的行。

UPDATE wms_task t
JOIN wms_allocation a ON a.id = t.allocation_id
SET t.detail_id = a.detail_id
WHERE t.task_type = 'PICK' AND t.detail_id = 0;