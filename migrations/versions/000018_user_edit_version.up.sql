-- Separate editor revision from credential revocation: changing a nickname
-- must invalidate stale editors without invalidating the user's login.
ALTER TABLE sys_user ADD COLUMN version BIGINT NOT NULL DEFAULT 1;
