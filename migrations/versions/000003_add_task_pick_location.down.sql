ALTER TABLE wms_task
    DROP KEY idx_task_location,
    DROP COLUMN batch_no,
    DROP COLUMN location_code,
    DROP COLUMN location_id;