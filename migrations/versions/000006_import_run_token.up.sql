-- 每次领取导入任务使用独立执行标识，防止旧 worker 覆盖重跑结果。
ALTER TABLE wms_import_task ADD COLUMN run_token VARCHAR(36) NOT NULL DEFAULT '';
