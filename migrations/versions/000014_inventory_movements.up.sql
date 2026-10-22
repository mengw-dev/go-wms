CREATE TABLE wms_inventory_movement (
 id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
 tenant_id BIGINT NOT NULL DEFAULT 0,
 operation_key VARCHAR(64) NOT NULL, request_hash VARCHAR(64) NOT NULL,
 move_no VARCHAR(64) NOT NULL, warehouse_id BIGINT NOT NULL, warehouse_code VARCHAR(32) NOT NULL,
 source_inventory_id BIGINT NOT NULL, target_inventory_id BIGINT NOT NULL,
 source_location_id BIGINT NOT NULL, target_location_id BIGINT NOT NULL,
 source_location_code VARCHAR(64) NOT NULL, target_location_code VARCHAR(64) NOT NULL,
 sku_id BIGINT NOT NULL, sku_code VARCHAR(64) NOT NULL, sku_name VARCHAR(128) NOT NULL, unit VARCHAR(16) NOT NULL,
 batch_no VARCHAR(64) NOT NULL, quantity INT NOT NULL,
 stock_in_time DATETIME(3) NOT NULL, operator VARCHAR(64) NOT NULL, reason VARCHAR(255) NOT NULL,
 created_at DATETIME(3) NOT NULL,
 CONSTRAINT chk_move_positive CHECK (quantity > 0),
 UNIQUE KEY uk_move_operation (tenant_id,operation_key),
 KEY idx_move_tenant_time (tenant_id,created_at)
) ENGINE=InnoDB;
