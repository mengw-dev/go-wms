-- Wave schema follows inventory alerts (000013) and inventory movements (000014).
-- Deploy after draining old application instances: old pickers do not understand wave_id.
ALTER TABLE wms_task ADD COLUMN wave_id BIGINT NOT NULL DEFAULT 0,
    ADD INDEX idx_wms_task_wave_id (wave_id);

CREATE TABLE wms_wave (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL, deleted_at DATETIME(3) NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    wave_no VARCHAR(64) NOT NULL, warehouse_id BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'CREATED', remark VARCHAR(255) NULL,
    created_by VARCHAR(64) NULL, claimed_by VARCHAR(64) NOT NULL DEFAULT '', lease_expire_at DATETIME(3) NULL,
    UNIQUE KEY uk_wave_no (tenant_id,wave_no),
    KEY idx_wave_tenant (tenant_id), KEY idx_wms_wave_warehouse_id (warehouse_id), KEY idx_wms_wave_deleted_at (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE wms_wave_order (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL, deleted_at DATETIME(3) NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0,
    wave_id BIGINT NOT NULL, order_id BIGINT NOT NULL, active_order_id BIGINT NULL,
    order_no VARCHAR(64) NOT NULL, slot_code VARCHAR(16) NOT NULL, shipped_at DATETIME(3) NULL,
    UNIQUE KEY uk_wave_active_order (tenant_id,active_order_id),
    KEY idx_wms_wave_order_wave_id (wave_id), KEY idx_wms_wave_order_order_id (order_id), KEY idx_wms_wave_order_deleted_at (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE wms_wave_line (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL, deleted_at DATETIME(3) NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0, wave_id BIGINT NOT NULL, inventory_id BIGINT NOT NULL,
    sku_id BIGINT NOT NULL, sku_code VARCHAR(64) NULL, barcode VARCHAR(64) NULL, sku_name VARCHAR(128) NULL,
    location_code VARCHAR(64) NULL, batch_no VARCHAR(64) NULL,
    target_qty BIGINT NOT NULL, picked_qty BIGINT NOT NULL DEFAULT 0,
    CONSTRAINT chk_wave_line_qty CHECK (target_qty > 0 AND picked_qty >= 0 AND picked_qty <= target_qty),
    UNIQUE KEY uk_wave_inventory (tenant_id,wave_id,inventory_id), KEY idx_wms_wave_line_wave_id (wave_id), KEY idx_wms_wave_line_deleted_at (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE wms_wave_item (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL, deleted_at DATETIME(3) NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0, wave_id BIGINT NOT NULL, wave_order_id BIGINT NOT NULL,
    line_id BIGINT NOT NULL, task_id BIGINT NOT NULL, allocation_id BIGINT NOT NULL,
    target_qty INT NOT NULL, sorted_qty INT NOT NULL DEFAULT 0,
    CONSTRAINT chk_wave_item_qty CHECK (target_qty > 0 AND sorted_qty >= 0 AND sorted_qty <= target_qty),
    UNIQUE KEY uk_wave_task (tenant_id,wave_id,task_id), KEY idx_wms_wave_item_wave_id (wave_id),
    KEY idx_wms_wave_item_wave_order_id (wave_order_id), KEY idx_wms_wave_item_line_id (line_id), KEY idx_wms_wave_item_deleted_at (deleted_at)
) ENGINE=InnoDB;
