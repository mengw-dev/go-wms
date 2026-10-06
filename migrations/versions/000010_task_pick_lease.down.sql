ALTER TABLE wms_task
    DROP COLUMN lease_expire_at,
    DROP COLUMN claim_token,
    DROP COLUMN claimed_by;