-- 在写入约束前，已有违反不变量或负数的库存行会使迁移失败。
-- 这是有意的 fail-fast：必须先审计并修复历史数据，不能静默修改库存。
ALTER TABLE wms_inventory
  ADD CONSTRAINT chk_inv_allocated_non_negative CHECK (allocated_quantity >= 0),
  ADD CONSTRAINT chk_inv_quantity_balance CHECK (stock_quantity = available_quantity + allocated_quantity);
