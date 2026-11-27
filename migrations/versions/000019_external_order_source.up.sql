-- 来源由服务端写入。历史 integration 操作者记录保守标记为外部来源，保留其业务号。
-- 开发模式可能已经 AutoMigrate 增加列；仍需执行下面的历史来源回填。
SET @source_ddl = IF(
    EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=DATABASE()
           AND table_name='wms_shipment_order' AND column_name='source'),
    'SELECT 1',
    'ALTER TABLE wms_shipment_order ADD COLUMN source VARCHAR(16) NOT NULL DEFAULT ''MANUAL'''
);
PREPARE source_stmt FROM @source_ddl;
EXECUTE source_stmt;
DEALLOCATE PREPARE source_stmt;
UPDATE wms_shipment_order SET source='INTEGRATION' WHERE created_by='integration';
