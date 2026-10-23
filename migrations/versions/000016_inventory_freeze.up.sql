-- Keep historical migrations immutable. The old balance check cannot permit frozen stock.
ALTER TABLE wms_inventory
 ADD COLUMN frozen_quantity INT NOT NULL DEFAULT 0,
 DROP CHECK chk_inv_quantity_balance,
 ADD CONSTRAINT chk_inv_quantity_balance_v2 CHECK (stock_quantity = ((available_quantity + allocated_quantity) + frozen_quantity)),
 ADD CONSTRAINT chk_inv_frozen_non_negative CHECK (frozen_quantity >= 0);
ALTER TABLE wms_inventory_trans
 ADD COLUMN frozen_before INT NOT NULL DEFAULT 0,
 ADD COLUMN frozen_after INT NOT NULL DEFAULT 0,
 ADD COLUMN reason VARCHAR(255) NOT NULL DEFAULT '';
