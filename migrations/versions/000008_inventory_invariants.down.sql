ALTER TABLE wms_inventory
  DROP CHECK chk_inv_quantity_balance,
  DROP CHECK chk_inv_allocated_non_negative;
