-- Stop application writers before rollback; old clients have no edit guard.
ALTER TABLE sys_user DROP COLUMN version;
