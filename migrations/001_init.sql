-- GoWMS 初始化迁移脚本（参考）
-- 实际表结构以 AutoMigrate 为准；此脚本用于人工审阅与生产环境 DBA 评审。
-- MySQL 8.0.16+（CHECK 约束强制执行）

CREATE DATABASE IF NOT EXISTS gowms DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE gowms;

-- ---------- 系统管理 ----------
CREATE TABLE IF NOT EXISTS sys_user (
  id            BIGINT PRIMARY KEY,
  username      VARCHAR(64)  NOT NULL,
  password_hash VARCHAR(128) NOT NULL,
  nickname      VARCHAR(64)  DEFAULT '',
  status        INT          DEFAULT 1,
  token_version INT          NOT NULL DEFAULT 1,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  UNIQUE KEY uk_user_username (username),
  KEY idx_sys_user_deleted_at (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS sys_role (
  id         BIGINT PRIMARY KEY,
  name       VARCHAR(64)  NOT NULL,
  perms      VARCHAR(1024) DEFAULT '',
  remark     VARCHAR(255) DEFAULT '',
  created_at DATETIME(3),
  updated_at DATETIME(3),
  deleted_at DATETIME(3),
  UNIQUE KEY uk_role_name (name),
  KEY idx_sys_role_deleted_at (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS sys_user_role (
  id      BIGINT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  role_id BIGINT NOT NULL,
  UNIQUE KEY uk_user_role (user_id, role_id)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS sys_oper_log (
  id         BIGINT PRIMARY KEY,
  user_id    BIGINT,
  username   VARCHAR(64),
  path       VARCHAR(255),
  method     VARCHAR(16),
  params     TEXT,
  ip         VARCHAR(64),
  cost_ms    BIGINT,
  status     INT,
  result     TEXT,
  created_at DATETIME(3),
  KEY idx_oper_log_created (created_at)
) ENGINE=InnoDB;

-- ---------- 基础资料 ----------
CREATE TABLE IF NOT EXISTS wms_warehouse (
  id         BIGINT PRIMARY KEY,
  code       VARCHAR(32) NOT NULL,
  name       VARCHAR(64) NOT NULL,
  remark     VARCHAR(255) DEFAULT '',
  status     INT DEFAULT 1,
  created_at DATETIME(3),
  updated_at DATETIME(3),
  deleted_at DATETIME(3),
  UNIQUE KEY uk_warehouse_code (code),
  KEY idx_wh_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_location (
  id           BIGINT PRIMARY KEY,
  warehouse_id BIGINT NOT NULL,
  code         VARCHAR(64) NOT NULL,   -- {库区}-{排}-{列}，如 A01-02-03
  zone         VARCHAR(32) DEFAULT '',
  status       INT DEFAULT 1,          -- 1 空闲 2 占用 0 禁用
  created_at   DATETIME(3),
  updated_at   DATETIME(3),
  deleted_at   DATETIME(3),
  KEY idx_loc_wh (warehouse_id),
  KEY idx_loc_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_sku (
  id         BIGINT PRIMARY KEY,
  code       VARCHAR(64)  NOT NULL,
  barcode    VARCHAR(64)  NOT NULL,
  name       VARCHAR(128) NOT NULL,
  spec       VARCHAR(128) DEFAULT '',
  unit       VARCHAR(16)  DEFAULT '',
  status     INT DEFAULT 1,
  created_at DATETIME(3),
  updated_at DATETIME(3),
  deleted_at DATETIME(3),
  UNIQUE KEY uk_sku_code (code),
  UNIQUE KEY uk_sku_barcode (barcode),
  KEY idx_sku_deleted (deleted_at)
) ENGINE=InnoDB;

-- ---------- 库存（核心） ----------
CREATE TABLE IF NOT EXISTS wms_inventory (
  id                 BIGINT PRIMARY KEY,
  warehouse_id       BIGINT NOT NULL,
  location_id        BIGINT NOT NULL,
  sku_id             BIGINT NOT NULL,
  batch_no           VARCHAR(64) NOT NULL DEFAULT '',
  stock_quantity     INT NOT NULL DEFAULT 0,
  available_quantity INT NOT NULL DEFAULT 0,
  allocated_quantity INT NOT NULL DEFAULT 0,
  stock_in_time      DATETIME(3),
  created_at         DATETIME(3),
  updated_at         DATETIME(3),
  deleted_at         DATETIME(3),
  UNIQUE KEY uk_inv (warehouse_id, location_id, sku_id, batch_no),
  CONSTRAINT chk_inv_non_negative CHECK (available_quantity >= 0 AND stock_quantity >= 0),
  KEY idx_inv_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_inventory_trans (
  id                BIGINT PRIMARY KEY,
  inventory_id      BIGINT NOT NULL,
  trans_type        VARCHAR(16) NOT NULL,   -- RECEIVE/ALLOCATE/SHIP/RELEASE/ADJUST
  quantity_change   INT NOT NULL,
  before_quantity   INT NOT NULL,
  after_quantity    INT NOT NULL,
  available_before  INT NOT NULL,
  available_after   INT NOT NULL,
  order_no          VARCHAR(64) DEFAULT '',
  task_no           VARCHAR(64) DEFAULT '',
  operator          VARCHAR(64) DEFAULT '',
  created_at        DATETIME(3),
  KEY idx_trans_inv (inventory_id),
  KEY idx_trans_type (trans_type),
  KEY idx_trans_order (order_no),
  KEY idx_trans_created (created_at)
) ENGINE=InnoDB;

-- ---------- 统一任务 ----------
CREATE TABLE IF NOT EXISTS wms_task (
  id           BIGINT PRIMARY KEY,
  task_no      VARCHAR(64) NOT NULL,
  task_type    VARCHAR(16) NOT NULL,   -- RECEIVE/PUTAWAY/PICK
  status       VARCHAR(16) NOT NULL DEFAULT 'CREATED',
  order_id     BIGINT NOT NULL,
  order_no     VARCHAR(64) DEFAULT '',
  detail_id    BIGINT DEFAULT 0,
  allocation_id BIGINT DEFAULT 0,
  sku_id       BIGINT NOT NULL,
  warehouse_id BIGINT NOT NULL,
  target_qty   INT NOT NULL,
  done_qty     INT NOT NULL DEFAULT 0,
  operator     VARCHAR(64) DEFAULT '',
  version      INT DEFAULT 1,
  created_at   DATETIME(3),
  updated_at   DATETIME(3),
  deleted_at   DATETIME(3),
  UNIQUE KEY uk_task_no (task_no),
  KEY idx_task_order (order_id),
  KEY idx_task_type (task_type),
  KEY idx_task_status (status),
  KEY idx_task_deleted (deleted_at)
) ENGINE=InnoDB;

-- ---------- 入库 ----------
CREATE TABLE IF NOT EXISTS wms_receipt_order (
  id            BIGINT PRIMARY KEY,
  order_no      VARCHAR(64) NOT NULL,
  warehouse_id  BIGINT NOT NULL,
  status        VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
  source        VARCHAR(16) DEFAULT 'MANUAL',
  remark        VARCHAR(255) DEFAULT '',
  expected_qty  INT NOT NULL DEFAULT 0,
  received_qty  INT NOT NULL DEFAULT 0,
  defective_qty INT NOT NULL DEFAULT 0,
  import_task_id VARCHAR(64) DEFAULT NULL,
  import_row    INT NOT NULL DEFAULT 0,
  created_by    VARCHAR(64) DEFAULT '',
  version       INT DEFAULT 1,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  UNIQUE KEY uk_receipt_no (order_no),
  UNIQUE KEY uk_import_row (import_task_id, import_row),
  KEY idx_ro_status (status),
  KEY idx_ro_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_receipt_order_detail (
  id            BIGINT PRIMARY KEY,
  order_id      BIGINT NOT NULL,
  sku_id        BIGINT NOT NULL,
  sku_code      VARCHAR(64) DEFAULT '',
  sku_name      VARCHAR(128) DEFAULT '',
  expected_qty  INT NOT NULL,
  received_qty  INT NOT NULL DEFAULT 0,
  defective_qty INT NOT NULL DEFAULT 0,
  batch_no      VARCHAR(64) DEFAULT '',
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  KEY idx_rod_order (order_id),
  KEY idx_rod_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_import_task (
  id           BIGINT PRIMARY KEY,
  task_id      VARCHAR(64) NOT NULL,
  status       VARCHAR(16) NOT NULL DEFAULT 'PENDING',
  file_name    VARCHAR(255) DEFAULT '',
  file_path    VARCHAR(255) DEFAULT '',
  total_rows   INT DEFAULT 0,
  success_rows INT DEFAULT 0,
  fail_rows    INT DEFAULT 0,
  error_msg    VARCHAR(1024) DEFAULT '',
  created_at   DATETIME(3),
  updated_at   DATETIME(3),
  deleted_at   DATETIME(3),
  UNIQUE KEY uk_import_task (task_id),
  KEY idx_import_status (status),
  KEY idx_import_updated (updated_at),
  KEY idx_import_deleted (deleted_at)
) ENGINE=InnoDB;

-- ---------- 出库 ----------
CREATE TABLE IF NOT EXISTS wms_shipment_order (
  id            BIGINT PRIMARY KEY,
  order_no      VARCHAR(64) NOT NULL,
  biz_order_no  VARCHAR(64) NOT NULL,   -- 幂等键：业务订单号
  warehouse_id  BIGINT NOT NULL,
  status        VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
  remark        VARCHAR(255) DEFAULT '',
  expected_qty  INT NOT NULL DEFAULT 0,
  allocated_qty INT NOT NULL DEFAULT 0,
  picked_qty    INT NOT NULL DEFAULT 0,
  created_by    VARCHAR(64) DEFAULT '',
  version       INT DEFAULT 1,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  UNIQUE KEY uk_shipment_no (order_no),
  UNIQUE KEY uk_shipment_biz (biz_order_no),
  KEY idx_so_status (status),
  KEY idx_so_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_shipment_order_detail (
  id            BIGINT PRIMARY KEY,
  order_id      BIGINT NOT NULL,
  sku_id        BIGINT NOT NULL,
  sku_code      VARCHAR(64) DEFAULT '',
  sku_name      VARCHAR(128) DEFAULT '',
  expected_qty  INT NOT NULL,
  allocated_qty INT NOT NULL DEFAULT 0,
  picked_qty    INT NOT NULL DEFAULT 0,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  KEY idx_sod_order (order_id),
  KEY idx_sod_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_allocation (
  id            BIGINT PRIMARY KEY,
  order_id      BIGINT NOT NULL,
  detail_id     BIGINT NOT NULL,
  inventory_id  BIGINT NOT NULL,
  sku_id        BIGINT NOT NULL,
  location_id   BIGINT NOT NULL,
  location_code VARCHAR(64) DEFAULT '',
  batch_no      VARCHAR(64) DEFAULT '',
  allocated_qty INT NOT NULL,
  picked_qty    INT NOT NULL DEFAULT 0,
  status        VARCHAR(16) NOT NULL DEFAULT 'ALLOCATED',
  version       INT DEFAULT 1,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  KEY idx_alloc_order (order_id),
  KEY idx_alloc_inv (inventory_id),
  KEY idx_alloc_status (status),
  KEY idx_alloc_deleted (deleted_at)
) ENGINE=InnoDB;

-- ---------- 盘点 ----------
CREATE TABLE IF NOT EXISTS wms_stocktake_order (
  id            BIGINT PRIMARY KEY,
  order_no      VARCHAR(64) NOT NULL,
  warehouse_id  BIGINT NOT NULL,
  location_id   BIGINT DEFAULT 0,
  location_code VARCHAR(64) DEFAULT '',
  status        VARCHAR(16) NOT NULL DEFAULT 'DRAFT',
  remark        VARCHAR(255) DEFAULT '',
  created_by    VARCHAR(64) DEFAULT '',
  version       INT DEFAULT 1,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  UNIQUE KEY uk_stocktake_no (order_no),
  KEY idx_sto_status (status),
  KEY idx_sto_deleted (deleted_at)
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS wms_stocktake_detail (
  id            BIGINT PRIMARY KEY,
  order_id      BIGINT NOT NULL,
  inventory_id  BIGINT NOT NULL,
  sku_id        BIGINT NOT NULL,
  sku_code      VARCHAR(64) DEFAULT '',
  sku_name      VARCHAR(128) DEFAULT '',
  location_id   BIGINT DEFAULT 0,
  location_code VARCHAR(64) DEFAULT '',
  batch_no      VARCHAR(64) DEFAULT '',
  book_qty      INT NOT NULL DEFAULT 0,
  actual_qty    INT DEFAULT NULL,
  diff_qty      INT NOT NULL DEFAULT 0,
  adjusted      TINYINT(1) NOT NULL DEFAULT 0,
  created_at    DATETIME(3),
  updated_at    DATETIME(3),
  deleted_at    DATETIME(3),
  KEY idx_std_order (order_id),
  KEY idx_std_inv (inventory_id),
  KEY idx_std_deleted (deleted_at)
) ENGINE=InnoDB;
