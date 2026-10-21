CREATE TABLE wms_inventory_alert_rule (
    id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
    created_at DATETIME(3) NULL, updated_at DATETIME(3) NULL, deleted_at DATETIME(3) NULL,
    tenant_id BIGINT NOT NULL DEFAULT 0, warehouse_id BIGINT NOT NULL, sku_id BIGINT NOT NULL,
    min_available_qty INT NOT NULL, enabled BOOLEAN NOT NULL, updated_by VARCHAR(64) NOT NULL,
    CONSTRAINT chk_alert_min_qty CHECK (min_available_qty > 0),
    UNIQUE KEY uk_alert_rule (tenant_id,warehouse_id,sku_id),
    KEY idx_wms_inventory_alert_rule_deleted_at (deleted_at)
) ENGINE=InnoDB;
