-- 000004 回滚：恢复原索引结构。

-- P2: 恢复被删的冗余单列索引
CREATE INDEX idx_task_type ON wms_task(task_type);
CREATE INDEX idx_task_status ON wms_task(status);
CREATE INDEX idx_trans_type ON wms_inventory_trans(trans_type);

-- P2: 删除新增的复合索引
DROP INDEX idx_trans_inv_type ON wms_inventory_trans;

-- P1: 删除补建的索引
DROP INDEX idx_user_role_role ON sys_user_role;
DROP INDEX idx_oper_log_username ON sys_oper_log;
DROP INDEX idx_import_stale ON wms_import_task;
DROP INDEX idx_task_sku ON wms_task;
DROP INDEX idx_task_allocation ON wms_task;
DROP INDEX idx_task_detail ON wms_task;
DROP INDEX idx_alloc_detail ON wms_allocation;
DROP INDEX uk_loc_wh_code ON wms_location;

-- P0: 删除核心索引
DROP INDEX idx_alloc_order_status ON wms_allocation;
DROP INDEX idx_sto_wh_status ON wms_stocktake_order;
DROP INDEX idx_so_wh_status ON wms_shipment_order;
DROP INDEX idx_ro_wh_status ON wms_receipt_order;
DROP INDEX idx_task_order_type_status ON wms_task;
DROP INDEX idx_inv_location ON wms_inventory;
DROP INDEX idx_inv_fifo ON wms_inventory;
