-- 停止业务写入并先运行 migrate up 的只读审计；不自动修正历史数量。
-- 每条 ALTER 仍由 MySQL 验证现有行；DDL 非跨表事务，失败后须检查 dirty 与已建立约束。
ALTER TABLE wms_task
  ADD CONSTRAINT chk_task_quantity_progress CHECK ((target_qty >= 0) AND (done_qty >= 0) AND (done_qty <= target_qty));
ALTER TABLE wms_allocation
  ADD CONSTRAINT chk_allocation_quantity_progress CHECK ((allocated_qty >= 0) AND (picked_qty >= 0) AND (picked_qty <= allocated_qty));
ALTER TABLE wms_shipment_order
  ADD CONSTRAINT chk_shipment_quantity_progress CHECK ((expected_qty >= 0) AND (allocated_qty >= 0) AND (picked_qty >= 0) AND (picked_qty <= allocated_qty) AND (allocated_qty <= expected_qty));
