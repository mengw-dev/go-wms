-- Business audit records must be exported/reconciled before destructive rollback.
-- Refuse rollback while either feature contains data.
ALTER TABLE wms_wave ADD CONSTRAINT chk_wave_rollback_no_abort CHECK (status <> 'ABORTED');
ALTER TABLE wms_work_exception ADD CONSTRAINT chk_exception_rollback_empty CHECK (id < 0);
ALTER TABLE wms_customer_return ADD CONSTRAINT chk_return_rollback_empty CHECK (id < 0);
DROP TABLE wms_customer_return;
DROP TABLE wms_work_exception;
ALTER TABLE wms_wave DROP CHECK chk_wave_rollback_no_abort;
ALTER TABLE wms_wave DROP COLUMN abort_reason, DROP COLUMN aborted_by, DROP COLUMN aborted_at;
