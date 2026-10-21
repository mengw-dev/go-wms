package app

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	invrepo "gowms/internal/modules/inventory/repository"
	"gowms/internal/pkg/tenant"
	"gowms/internal/testutil"
)

// Release DDL, mixed tenant/SKU rows, read-only plans: a tiny one-SKU fixture
// cannot demonstrate whether the deployed FIFO index is selective.
func TestInventoryScalePlan(t *testing.T) {
	dsn := os.Getenv("WMS_TEST_DSN")
	if dsn == "" {
		dsn = "root:1234@tcp(127.0.0.1:3306)/gowms?parseTime=true&timeout=2s"
	}
	db := testutil.OpenMigratedMySQL(t, dsn)
	ctx, cancel := context.WithTimeout(tenant.WithTenant(context.Background(), 81), 30*time.Second)
	defer cancel()
	// 10,000 rows spread across 10 tenants and 100 SKUs; the target has 100.
	for start := 0; start < 10000; start += 200 {
		var sql strings.Builder
		sql.WriteString("INSERT INTO wms_inventory (id,tenant_id,warehouse_id,location_id,sku_id,batch_no,stock_quantity,available_quantity,allocated_quantity,stock_in_time,created_at,updated_at) VALUES ")
		args := make([]any, 0, 200*7)
		for n := start; n < start+200; n++ {
			if n > start {
				sql.WriteByte(',')
			}
			sql.WriteString("(?,?,1,?,?,?,1,1,0,?,NOW(),NOW())")
			args = append(args, n+1, 81+n%10, n+1, 1+n%100, "PLAN", time.Unix(int64(n+1), 0))
		}
		if err := db.WithContext(ctx).Exec(sql.String(), args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.WithContext(ctx).Exec("ANALYZE TABLE wms_inventory").Error; err != nil {
		t.Fatal(err)
	}
	candidates, err := invrepo.New().ListFIFOCandidates(db.WithContext(ctx), 1, 1, 20, time.Time{}, 0)
	if err != nil || len(candidates) != 20 {
		t.Fatalf("candidates=%d err=%v", len(candidates), err)
	}
	next, err := invrepo.New().ListFIFOCandidates(db.WithContext(ctx), 1, 1, 20, candidates[19].StockInTime, candidates[19].ID)
	if err != nil || len(next) != 20 || next[0].ID <= candidates[19].ID {
		t.Fatalf("keyset err=%v rows=%d", err, len(next))
	}
	for _, keyset := range []bool{false, true} {
		query := "SELECT id, available_quantity, stock_in_time FROM wms_inventory WHERE tenant_id = 81 AND warehouse_id = 1 AND sku_id = 1 AND available_quantity > 0 AND deleted_at IS NULL"
		args := []any{}
		if keyset {
			query += " AND (stock_in_time > ? OR (stock_in_time = ? AND id > ?))"
			args = append(args, candidates[19].StockInTime, candidates[19].StockInTime, candidates[19].ID)
		}
		query += " ORDER BY stock_in_time, id LIMIT 20"
		for _, format := range []string{"EXPLAIN FORMAT=TREE ", "EXPLAIN ANALYZE "} {
			var plan []struct {
				Plan string `gorm:"column:EXPLAIN"`
			}
			if err := db.WithContext(ctx).Raw(format+query, args...).Scan(&plan).Error; err != nil {
				t.Fatal(err)
			}
			if len(plan) == 0 {
				t.Fatal("empty plan")
			}
			for _, row := range plan {
				t.Logf("RELEASE_FIFO_PLAN keyset=%v %s", keyset, row.Plan)
			}
		}
	}
}
