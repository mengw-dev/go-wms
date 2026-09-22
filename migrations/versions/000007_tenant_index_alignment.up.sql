-- 000007: 统一多租户查询索引，把 tenant_id 前置到高频复合索引。
-- 只调整索引，不修改业务数据和字段语义。

-- 库位的业务唯一性必须在租户内判断；仓库 ID 虽为全局 ID，仍保持索引前缀一致。
ALTER TABLE wms_location DROP INDEX uk_loc_wh_code,
  ADD UNIQUE KEY uk_loc_wh_code (tenant_id, warehouse_id, code);

-- 用户和角色 ID 为全局 ID，但角色删除检查按租户查询，唯一键和辅助索引都保留租户前缀。
ALTER TABLE sys_user_role DROP INDEX uk_user_role,
  ADD UNIQUE KEY uk_user_role (tenant_id, user_id, role_id);

-- 库存 FIFO 与删除引用检查。
DROP INDEX idx_inv_fifo ON wms_inventory;
CREATE INDEX idx_inv_tenant_fifo
  ON wms_inventory (tenant_id, warehouse_id, sku_id, available_quantity, stock_in_time);
DROP INDEX idx_inv_location ON wms_inventory;
CREATE INDEX idx_inv_tenant_location ON wms_inventory (tenant_id, location_id);
CREATE INDEX idx_inv_tenant_sku ON wms_inventory (tenant_id, sku_id);

-- 任务推进、取消和基础资料引用检查。
DROP INDEX idx_task_order_type_status ON wms_task;
CREATE INDEX idx_task_tenant_order_type_status
  ON wms_task (tenant_id, order_id, task_type, status);
DROP INDEX idx_task_location ON wms_task;
CREATE INDEX idx_task_tenant_location ON wms_task (tenant_id, location_id);
DROP INDEX idx_task_detail ON wms_task;
CREATE INDEX idx_task_tenant_detail ON wms_task (tenant_id, detail_id);
DROP INDEX idx_task_allocation ON wms_task;
CREATE INDEX idx_task_tenant_allocation ON wms_task (tenant_id, allocation_id);
DROP INDEX idx_task_sku ON wms_task;
CREATE INDEX idx_task_tenant_sku ON wms_task (tenant_id, sku_id);
CREATE INDEX idx_task_tenant_warehouse ON wms_task (tenant_id, warehouse_id);

-- 单据列表和明细引用检查。
DROP INDEX idx_ro_wh_status ON wms_receipt_order;
CREATE INDEX idx_ro_tenant_wh_status ON wms_receipt_order (tenant_id, warehouse_id, status);
CREATE INDEX idx_rod_tenant_order ON wms_receipt_order_detail (tenant_id, order_id);
CREATE INDEX idx_rod_tenant_sku ON wms_receipt_order_detail (tenant_id, sku_id);

DROP INDEX idx_so_wh_status ON wms_shipment_order;
CREATE INDEX idx_so_tenant_wh_status ON wms_shipment_order (tenant_id, warehouse_id, status);
CREATE INDEX idx_sod_tenant_order ON wms_shipment_order_detail (tenant_id, order_id);
CREATE INDEX idx_sod_tenant_sku ON wms_shipment_order_detail (tenant_id, sku_id);

DROP INDEX idx_alloc_order_status ON wms_allocation;
CREATE INDEX idx_alloc_tenant_order_status ON wms_allocation (tenant_id, order_id, status);
DROP INDEX idx_alloc_detail ON wms_allocation;
CREATE INDEX idx_alloc_tenant_detail ON wms_allocation (tenant_id, detail_id);
CREATE INDEX idx_alloc_tenant_sku ON wms_allocation (tenant_id, sku_id);
CREATE INDEX idx_alloc_tenant_location ON wms_allocation (tenant_id, location_id);

DROP INDEX idx_sto_wh_status ON wms_stocktake_order;
CREATE INDEX idx_sto_tenant_wh_status ON wms_stocktake_order (tenant_id, warehouse_id, status);
CREATE INDEX idx_std_tenant_order ON wms_stocktake_detail (tenant_id, order_id);
CREATE INDEX idx_std_tenant_inv ON wms_stocktake_detail (tenant_id, inventory_id);
CREATE INDEX idx_std_tenant_sku ON wms_stocktake_detail (tenant_id, sku_id);

-- 流水、操作日志和角色删除检查。
DROP INDEX idx_trans_inv_type ON wms_inventory_trans;
CREATE INDEX idx_trans_tenant_inv_type
  ON wms_inventory_trans (tenant_id, inventory_id, trans_type);
DROP INDEX idx_oper_log_username ON sys_oper_log;
CREATE INDEX idx_oper_log_tenant_username ON sys_oper_log (tenant_id, username, created_at);
DROP INDEX idx_user_role_role ON sys_user_role;
CREATE INDEX idx_user_role_tenant_role ON sys_user_role (tenant_id, role_id);
