-- 000007 回滚：恢复多租户改造前的索引结构。
-- 仅回滚索引，不删除 tenant_id 列或业务数据。

ALTER TABLE wms_location DROP INDEX uk_loc_wh_code,
  ADD UNIQUE KEY uk_loc_wh_code (warehouse_id, code);
ALTER TABLE sys_user_role DROP INDEX uk_user_role,
  ADD UNIQUE KEY uk_user_role (user_id, role_id);

DROP INDEX idx_inv_tenant_fifo ON wms_inventory;
CREATE INDEX idx_inv_fifo ON wms_inventory (warehouse_id, sku_id, available_quantity, stock_in_time);
DROP INDEX idx_inv_tenant_location ON wms_inventory;
CREATE INDEX idx_inv_location ON wms_inventory (location_id);
DROP INDEX idx_inv_tenant_sku ON wms_inventory;

DROP INDEX idx_task_tenant_order_type_status ON wms_task;
CREATE INDEX idx_task_order_type_status ON wms_task (order_id, task_type, status);
DROP INDEX idx_task_tenant_location ON wms_task;
CREATE INDEX idx_task_location ON wms_task (location_id);
DROP INDEX idx_task_tenant_detail ON wms_task;
CREATE INDEX idx_task_detail ON wms_task (detail_id);
DROP INDEX idx_task_tenant_allocation ON wms_task;
CREATE INDEX idx_task_allocation ON wms_task (allocation_id);
DROP INDEX idx_task_tenant_sku ON wms_task;
CREATE INDEX idx_task_sku ON wms_task (sku_id);
DROP INDEX idx_task_tenant_warehouse ON wms_task;

DROP INDEX idx_ro_tenant_wh_status ON wms_receipt_order;
CREATE INDEX idx_ro_wh_status ON wms_receipt_order (warehouse_id, status);
DROP INDEX idx_rod_tenant_order ON wms_receipt_order_detail;
DROP INDEX idx_rod_tenant_sku ON wms_receipt_order_detail;

DROP INDEX idx_so_tenant_wh_status ON wms_shipment_order;
CREATE INDEX idx_so_wh_status ON wms_shipment_order (warehouse_id, status);
DROP INDEX idx_sod_tenant_order ON wms_shipment_order_detail;
DROP INDEX idx_sod_tenant_sku ON wms_shipment_order_detail;

DROP INDEX idx_alloc_tenant_order_status ON wms_allocation;
CREATE INDEX idx_alloc_order_status ON wms_allocation (order_id, status);
DROP INDEX idx_alloc_tenant_detail ON wms_allocation;
CREATE INDEX idx_alloc_detail ON wms_allocation (detail_id);
DROP INDEX idx_alloc_tenant_sku ON wms_allocation;
DROP INDEX idx_alloc_tenant_location ON wms_allocation;

DROP INDEX idx_sto_tenant_wh_status ON wms_stocktake_order;
CREATE INDEX idx_sto_wh_status ON wms_stocktake_order (warehouse_id, status);
DROP INDEX idx_std_tenant_order ON wms_stocktake_detail;
DROP INDEX idx_std_tenant_inv ON wms_stocktake_detail;
DROP INDEX idx_std_tenant_sku ON wms_stocktake_detail;

DROP INDEX idx_trans_tenant_inv_type ON wms_inventory_trans;
CREATE INDEX idx_trans_inv_type ON wms_inventory_trans (inventory_id, trans_type);
DROP INDEX idx_oper_log_tenant_username ON sys_oper_log;
CREATE INDEX idx_oper_log_username ON sys_oper_log (username, created_at);
DROP INDEX idx_user_role_tenant_role ON sys_user_role;
CREATE INDEX idx_user_role_role ON sys_user_role (role_id);
