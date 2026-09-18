-- 000005: 多租户最小版（共享库加列方案）。
-- 全部业务表新增 tenant_id 列（存量数据默认 0 = 默认租户，完全向后兼容单租户现状）；
-- 唯一键改为含 tenant_id 的联合唯一，保证各租户编码空间独立；
-- 高频查询表补 tenant_id 普通索引。
-- 纯加法迁移（唯一键先删后建），不修改已有列定义与既有索引。

-- ---------- 1) 18 张表全部加 tenant_id ----------
ALTER TABLE sys_user          ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sys_role          ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sys_user_role     ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE sys_oper_log      ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_warehouse     ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_location      ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_sku           ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_inventory     ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_inventory_trans ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_task          ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_receipt_order ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_receipt_order_detail ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_import_task   ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_shipment_order ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_shipment_order_detail ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_allocation    ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_stocktake_order ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE wms_stocktake_detail ADD COLUMN tenant_id BIGINT NOT NULL DEFAULT 0;

-- ---------- 2) 唯一键改联合唯一（tenant_id 前置） ----------
-- 同名用户/角色/编码在各租户内独立；跨租户重复由 (tenant_id, 原列) 联合唯一约束。
ALTER TABLE sys_user      DROP INDEX uk_user_username,  ADD UNIQUE KEY uk_user_username  (tenant_id, username);
ALTER TABLE sys_role      DROP INDEX uk_role_name,      ADD UNIQUE KEY uk_role_name      (tenant_id, name);
ALTER TABLE wms_warehouse DROP INDEX uk_warehouse_code, ADD UNIQUE KEY uk_warehouse_code (tenant_id, code);
ALTER TABLE wms_sku       DROP INDEX uk_sku_code,       ADD UNIQUE KEY uk_sku_code       (tenant_id, code);
ALTER TABLE wms_sku       DROP INDEX uk_sku_barcode,    ADD UNIQUE KEY uk_sku_barcode    (tenant_id, barcode);
ALTER TABLE wms_inventory DROP INDEX uk_inv,
  ADD UNIQUE KEY uk_inv (tenant_id, warehouse_id, location_id, sku_id, batch_no);
ALTER TABLE wms_task      DROP INDEX uk_task_no,        ADD UNIQUE KEY uk_task_no        (tenant_id, task_no);
ALTER TABLE wms_receipt_order DROP INDEX uk_receipt_no, ADD UNIQUE KEY uk_receipt_no    (tenant_id, order_no);
ALTER TABLE wms_receipt_order DROP INDEX uk_import_row,
  ADD UNIQUE KEY uk_import_row (tenant_id, import_task_id, import_row);
ALTER TABLE wms_shipment_order DROP INDEX uk_shipment_no, ADD UNIQUE KEY uk_shipment_no (tenant_id, order_no);
ALTER TABLE wms_shipment_order DROP INDEX uk_shipment_biz, ADD UNIQUE KEY uk_shipment_biz (tenant_id, biz_order_no);
ALTER TABLE wms_stocktake_order DROP INDEX uk_stocktake_no, ADD UNIQUE KEY uk_stocktake_no (tenant_id, order_no);
ALTER TABLE wms_import_task DROP INDEX uk_import_task,  ADD UNIQUE KEY uk_import_task   (tenant_id, task_id);

-- ---------- 3) 高频查询表补 tenant_id 索引 ----------
-- 注：已改为联合唯一的表（如 sys_user/wms_sku/wms_task 等）其联合唯一索引本身
-- 即以 tenant_id 前置，可服务按租户过滤，无需重复单列索引。
CREATE INDEX idx_inv_tenant      ON wms_inventory(tenant_id);
CREATE INDEX idx_trans_tenant    ON wms_inventory_trans(tenant_id);
CREATE INDEX idx_oper_log_tenant ON sys_oper_log(tenant_id);
CREATE INDEX idx_wh_tenant       ON wms_warehouse(tenant_id);
CREATE INDEX idx_loc_tenant      ON wms_location(tenant_id);
CREATE INDEX idx_task_tenant     ON wms_task(tenant_id);
CREATE INDEX idx_ro_tenant       ON wms_receipt_order(tenant_id);
CREATE INDEX idx_so_tenant       ON wms_shipment_order(tenant_id);
CREATE INDEX idx_sto_tenant      ON wms_stocktake_order(tenant_id);
CREATE INDEX idx_alloc_tenant    ON wms_allocation(tenant_id);
CREATE INDEX idx_import_tenant   ON wms_import_task(tenant_id);
