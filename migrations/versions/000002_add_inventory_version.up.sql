-- 000002: 为 wms_inventory 补充乐观锁版本列（Inventory 模型嵌入 Versioned，000001 建表脚本遗漏）。
ALTER TABLE wms_inventory
    ADD COLUMN version INT NOT NULL DEFAULT 1 AFTER deleted_at;
