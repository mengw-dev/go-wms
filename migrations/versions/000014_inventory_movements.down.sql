-- Never discard receipts after stock has moved. Empty development databases can
-- roll back; populated installations require an explicit forward migration.
CREATE TEMPORARY TABLE _wms_move_down_guard (n BIGINT, CONSTRAINT chk_movement_history_empty CHECK (n = 0));
INSERT INTO _wms_move_down_guard SELECT COUNT(*) FROM wms_inventory_movement;
DROP TEMPORARY TABLE _wms_move_down_guard;
DROP TABLE wms_inventory_movement;
