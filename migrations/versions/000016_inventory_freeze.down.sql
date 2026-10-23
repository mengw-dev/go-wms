-- Refuse rollback when frozen stock exists; never silently unfreeze business data.
ALTER TABLE wms_inventory_trans ADD CONSTRAINT chk_v16_rollback_legacy CHECK (trans_type NOT IN ('FREEZE','UNFREEZE','SHORTAGE'));
ALTER TABLE wms_inventory_trans DROP CHECK chk_v16_rollback_legacy;
ALTER TABLE wms_inventory ADD CONSTRAINT chk_inv_quantity_balance CHECK (stock_quantity = (available_quantity + allocated_quantity));
ALTER TABLE wms_inventory
 DROP CHECK chk_inv_quantity_balance_v2,
 DROP CHECK chk_inv_frozen_non_negative,
 DROP COLUMN frozen_quantity;
ALTER TABLE wms_inventory_trans DROP COLUMN frozen_before, DROP COLUMN frozen_after, DROP COLUMN reason;
