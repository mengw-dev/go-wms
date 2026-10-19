ALTER TABLE wms_shipment_order DROP CHECK chk_shipment_quantity_progress;
ALTER TABLE wms_allocation DROP CHECK chk_allocation_quantity_progress;
ALTER TABLE wms_task DROP CHECK chk_task_quantity_progress;
