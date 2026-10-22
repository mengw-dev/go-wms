-- Rollback is allowed only before any wave history exists. Never discard staged work.
CREATE TEMPORARY TABLE wave_rollback_guard (wave_count BIGINT CHECK (wave_count = 0));
INSERT INTO wave_rollback_guard SELECT COUNT(*) FROM wms_wave;
DROP TEMPORARY TABLE wave_rollback_guard;
DROP TABLE wms_wave_item;
DROP TABLE wms_wave_line;
DROP TABLE wms_wave_order;
DROP TABLE wms_wave;
ALTER TABLE wms_task DROP INDEX idx_wms_task_wave_id, DROP COLUMN wave_id;
