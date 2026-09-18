-- 000005 down: 回滚多租户列与联合唯一键。
-- 注意：回滚前必须保证各业务编码在全局唯一（不同租户存在同名编码时会失败）。

ALTER TABLE sys_user      DROP INDEX uk_user_username,  ADD UNIQUE KEY uk_user_username  (username);
ALTER TABLE sys_role      DROP INDEX uk_role_name,      ADD UNIQUE KEY uk_role_name      (name);
ALTER TABLE wms_warehouse DROP INDEX uk_warehouse_code, ADD UNIQUE KEY uk_warehouse_code (code);
ALTER TABLE wms_sku       DROP INDEX uk_sku_code,       ADD UNIQUE KEY uk_sku_code       (code);
ALTER TABLE wms_sku       DROP INDEX uk_sku_barcode,    ADD UNIQUE KEY uk_sku_barcode    (barcode);
ALTER TABLE wms_inventory DROP INDEX uk_inv,
  ADD UNIQUE KEY uk_inv (warehouse_id, location_id, sku_id, batch_no);
ALTER TABLE wms_task      DROP INDEX uk_task_no,        ADD UNIQUE KEY uk_task_no        (task_no);
ALTER TABLE wms_receipt_order DROP INDEX uk_receipt_no, ADD UNIQUE KEY uk_receipt_no    (order_no);
ALTER TABLE wms_receipt_order DROP INDEX uk_import_row,
  ADD UNIQUE KEY uk_import_row (import_task_id, import_row);
ALTER TABLE wms_shipment_order DROP INDEX uk_shipment_no, ADD UNIQUE KEY uk_shipment_no (order_no);
ALTER TABLE wms_shipment_order DROP INDEX uk_shipment_biz, ADD UNIQUE KEY uk_shipment_biz (biz_order_no);
ALTER TABLE wms_stocktake_order DROP INDEX uk_stocktake_no, ADD UNIQUE KEY uk_stocktake_no (order_no);
ALTER TABLE wms_import_task DROP INDEX uk_import_task,  ADD UNIQUE KEY uk_import_task   (task_id);

DROP INDEX idx_inv_tenant      ON wms_inventory;
DROP INDEX idx_trans_tenant    ON wms_inventory_trans;
DROP INDEX idx_oper_log_tenant ON sys_oper_log;
DROP INDEX idx_wh_tenant       ON wms_warehouse;
DROP INDEX idx_loc_tenant      ON wms_location;
DROP INDEX idx_task_tenant     ON wms_task;
DROP INDEX idx_ro_tenant       ON wms_receipt_order;
DROP INDEX idx_so_tenant       ON wms_shipment_order;
DROP INDEX idx_sto_tenant      ON wms_stocktake_order;
DROP INDEX idx_alloc_tenant    ON wms_allocation;
DROP INDEX idx_import_tenant   ON wms_import_task;

ALTER TABLE sys_user            DROP COLUMN tenant_id;
ALTER TABLE sys_role            DROP COLUMN tenant_id;
ALTER TABLE sys_user_role       DROP COLUMN tenant_id;
ALTER TABLE sys_oper_log        DROP COLUMN tenant_id;
ALTER TABLE wms_warehouse       DROP COLUMN tenant_id;
ALTER TABLE wms_location        DROP COLUMN tenant_id;
ALTER TABLE wms_sku             DROP COLUMN tenant_id;
ALTER TABLE wms_inventory       DROP COLUMN tenant_id;
ALTER TABLE wms_inventory_trans DROP COLUMN tenant_id;
ALTER TABLE wms_task            DROP COLUMN tenant_id;
ALTER TABLE wms_receipt_order   DROP COLUMN tenant_id;
ALTER TABLE wms_receipt_order_detail DROP COLUMN tenant_id;
ALTER TABLE wms_import_task     DROP COLUMN tenant_id;
ALTER TABLE wms_shipment_order  DROP COLUMN tenant_id;
ALTER TABLE wms_shipment_order_detail DROP COLUMN tenant_id;
ALTER TABLE wms_allocation      DROP COLUMN tenant_id;
ALTER TABLE wms_stocktake_order DROP COLUMN tenant_id;
ALTER TABLE wms_stocktake_detail DROP COLUMN tenant_id;
