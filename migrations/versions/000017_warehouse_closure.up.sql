CREATE TABLE wms_work_exception (
 id BIGINT NOT NULL PRIMARY KEY,
 tenant_id BIGINT NOT NULL DEFAULT 0,
 order_id BIGINT NOT NULL, order_no VARCHAR(64) NOT NULL,
 task_id BIGINT NOT NULL, inventory_id BIGINT NOT NULL,
 kind VARCHAR(16) NOT NULL, quantity INT NOT NULL,
 reason VARCHAR(255) NOT NULL, status VARCHAR(16) NOT NULL DEFAULT 'OPEN',
 resolution VARCHAR(255) NOT NULL DEFAULT '', operator VARCHAR(64) NOT NULL,
 resolved_by VARCHAR(64) NOT NULL DEFAULT '', created_at DATETIME(3), resolved_at DATETIME(3),
 KEY idx_exception_tenant_order (tenant_id,order_id),
 CONSTRAINT chk_exception_quantity CHECK (quantity > 0)
) ENGINE=InnoDB;
CREATE TABLE wms_customer_return (
 id BIGINT NOT NULL PRIMARY KEY,
 tenant_id BIGINT NOT NULL DEFAULT 0,
 return_no VARCHAR(64) NOT NULL, order_id BIGINT NOT NULL, order_no VARCHAR(64) NOT NULL,
 allocation_id BIGINT NOT NULL, warehouse_id BIGINT NOT NULL, location_id BIGINT NOT NULL,
 sku_id BIGINT NOT NULL, batch_no VARCHAR(64) NOT NULL,
 sku_code VARCHAR(64) NOT NULL, sku_name VARCHAR(128) NOT NULL, unit VARCHAR(16) NOT NULL, location_code VARCHAR(64) NOT NULL,
 good_qty INT NOT NULL, bad_qty INT NOT NULL,
 reason VARCHAR(255) NOT NULL, operator VARCHAR(64) NOT NULL, created_at DATETIME(3),
 KEY idx_return_tenant_order (tenant_id,order_id), KEY idx_return_allocation (allocation_id),
 CONSTRAINT chk_return_quantity CHECK (good_qty >= 0 AND bad_qty >= 0 AND good_qty + bad_qty > 0 AND good_qty + bad_qty <= 2147483647)
) ENGINE=InnoDB;
ALTER TABLE wms_wave ADD COLUMN abort_reason VARCHAR(255) NOT NULL DEFAULT '', ADD COLUMN aborted_by VARCHAR(64) NOT NULL DEFAULT '', ADD COLUMN aborted_at DATETIME(3);
