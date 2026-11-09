package migrations_test

import (
	"reflect"
	"testing"
)

func TestUserEditRevisionUpgradeRollbackPreservesAccounts(t *testing.T) {
	db, m := checkMigrator(t)
	if err := m.Migrate(17); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sys_user (id,tenant_id,username,password_hash,nickname,status,token_version,deleted_at)
		VALUES (901,11,'active','preserved-hash','kept',0,9,NULL),
		(902,12,'deleted','deleted-hash','history',1,7,'2026-01-01 00:00:00')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sys_role(id,tenant_id,name,perms) VALUES(901,11,'operator','wms:basic')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO sys_user_role(tenant_id,user_id,role_id) VALUES(11,901,901)`).Error; err != nil {
		t.Fatal(err)
	}
	type snapshot struct {
		ID, TenantID                              int64
		Username, PasswordHash, Nickname, Deleted string
		Status, TokenVersion                      int
	}
	read := func() []snapshot {
		t.Helper()
		var rows []snapshot
		if err := db.Raw(`SELECT id,tenant_id,username,password_hash,nickname,status,token_version,COALESCE(CAST(deleted_at AS CHAR),'') AS deleted FROM sys_user ORDER BY id`).Scan(&rows).Error; err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := read()
	for _, version := range []uint{18, 17, 18} {
		if err := m.Migrate(version); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, read()) {
			t.Fatalf("migration to %d changed account data", version)
		}
		var links int64
		if err := db.Raw("SELECT COUNT(*) FROM sys_user_role WHERE tenant_id=11 AND user_id=901 AND role_id=901").Scan(&links).Error; err != nil || links != 1 {
			t.Fatalf("links changed: %d %v", links, err)
		}
		if db.Migrator().HasColumn("sys_user", "version") != (version == 18) {
			t.Fatalf("wrong column state at %d", version)
		}
		if version == 18 {
			var revisions []int
			if err := db.Raw("SELECT version FROM sys_user ORDER BY id").Scan(&revisions).Error; err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(revisions, []int{1, 1}) {
				t.Fatalf("existing account defaults = %v", revisions)
			}
		}
	}
	version, dirty, err := m.Version()
	if err != nil || dirty || version != 18 {
		t.Fatalf("migration state: %d dirty=%v err=%v", version, dirty, err)
	}
}
