-- 000003: 拣货任务补充作业位置（库位 + 批次），拣货员可直达库位并按批次核对，避免同库位不同批次拣错。
ALTER TABLE wms_task
    ADD COLUMN location_id BIGINT DEFAULT 0 AFTER warehouse_id,
    ADD COLUMN location_code VARCHAR(64) DEFAULT '' AFTER location_id,
    ADD COLUMN batch_no VARCHAR(64) DEFAULT '' AFTER location_code,
    ADD KEY idx_task_location (location_id);