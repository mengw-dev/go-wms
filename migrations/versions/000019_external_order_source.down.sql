-- 回退会失去外部单据删除保护；必须先停止 OMS 下发并完成在途请求核对。
ALTER TABLE wms_shipment_order DROP COLUMN source;
