-- 000004: 补建核心查询索引，消除高频路径全表扫描。
-- 纯加法迁移，不修改已有索引和列定义。

-- P0: FIFO 分配查询（warehouse_id + sku_id + available_quantity > 0 ORDER BY stock_in_time）
CREATE INDEX idx_inv_fifo ON wms_inventory(warehouse_id, sku_id, available_quantity, stock_in_time);

-- P0: 库位删除校验（HasStockByLocation 按 location_id 查）
CREATE INDEX idx_inv_location ON wms_inventory(location_id);

-- P0: 任务推进查询（order_id + task_type + status 组合过滤）
CREATE INDEX idx_task_order_type_status ON wms_task(order_id, task_type, status);

-- P0: 单据列表查询（warehouse_id + status 过滤）
CREATE INDEX idx_ro_wh_status ON wms_receipt_order(warehouse_id, status);
CREATE INDEX idx_so_wh_status ON wms_shipment_order(warehouse_id, status);
CREATE INDEX idx_sto_wh_status ON wms_stocktake_order(warehouse_id, status);

-- P0: 取消出库时释放分配行（order_id + status='ALLOCATED'）
CREATE INDEX idx_alloc_order_status ON wms_allocation(order_id, status);

-- P1: GORM tag 声明但 SQL 脚本遗漏的索引
CREATE UNIQUE INDEX uk_loc_wh_code ON wms_location(warehouse_id, code);
CREATE INDEX idx_alloc_detail ON wms_allocation(detail_id);
CREATE INDEX idx_task_detail ON wms_task(detail_id);
CREATE INDEX idx_task_allocation ON wms_task(allocation_id);
CREATE INDEX idx_task_sku ON wms_task(sku_id);

-- P1: 悬挂导入任务补偿扫描（status + updated_at）
CREATE INDEX idx_import_stale ON wms_import_task(status, updated_at);

-- P1: 操作日志按用户名查询
CREATE INDEX idx_oper_log_username ON sys_oper_log(username, created_at);

-- P1: 角色删除前校验引用（role_id 单列）
CREATE INDEX idx_user_role_role ON sys_user_role(role_id);

-- P2: 库存流水按 inventory_id + trans_type 组合查询，替换低区分度单列索引
CREATE INDEX idx_trans_inv_type ON wms_inventory_trans(inventory_id, trans_type);
DROP INDEX idx_trans_type ON wms_inventory_trans;

-- P2: 任务复合索引覆盖单列，删除冗余
DROP INDEX idx_task_type ON wms_task;
DROP INDEX idx_task_status ON wms_task;
