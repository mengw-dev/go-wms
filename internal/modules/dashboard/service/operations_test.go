package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"gowms/internal/modules/dashboard/dto"
	inboundmodel "gowms/internal/modules/inbound/model"
	invmodel "gowms/internal/modules/inventory/model"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
)

func TestOperationsUseLedgerDatesTenantAndWarehouse(t *testing.T) {
	// Reproduce the CI driver's UTC setting even on a China-time developer PC.
	// The fixture must preserve business-clock DATETIME values independently.
	cfg := dashboardDSN(t)
	cfg.Loc = time.UTC
	t.Setenv("WMS_TEST_DSN", cfg.FormatDSN())
	db, svc, now := fixture(t)
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, now.Location())
	for _, tid := range []int64{0, 99, 77} {
		q := db.WithContext(tenant.WithExactTenant(context.Background(), tid))
		var stock, other, deleted invmodel.Inventory
		if err := q.Where("warehouse_id=1").First(&stock).Error; err != nil {
			t.Fatal(err)
		}
		if err := q.Where("warehouse_id=2").First(&other).Error; err != nil {
			t.Fatal(err)
		}
		if err := q.Unscoped().Where("batch_no='DELETED'").First(&deleted).Error; err != nil {
			t.Fatal(err)
		}
		for _, row := range []invmodel.InventoryTrans{
			{InventoryID: stock.ID, TransType: "RECEIVE", CreatedAt: day, OrderNo: "RK1"},
			{InventoryID: stock.ID, TransType: "RECEIVE", CreatedAt: now, OrderNo: "RK1"},
			{InventoryID: stock.ID, TransType: "SHIP", CreatedAt: now.Add(-time.Minute), OrderNo: "CK1"},
			{InventoryID: stock.ID, TransType: "RECEIVE", CreatedAt: day.Add(-time.Second)},
			{InventoryID: stock.ID, TransType: "SHIP", CreatedAt: now.Add(time.Second)},
			{InventoryID: stock.ID, TransType: "OPENING", CreatedAt: now},
			{InventoryID: other.ID, TransType: "SHIP", CreatedAt: now},
			{InventoryID: deleted.ID, TransType: "RECEIVE", CreatedAt: now},
			{InventoryID: 999999, TransType: "RECEIVE", CreatedAt: now},
		} {
			if err := q.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
		for i, row := range []inboundmodel.ImportTask{
			{Status: "FAILED", Base: modelbase.Base{CreatedAt: day.Add(-6 * 24 * time.Hour)}, FilePath: "must-not-leak", RunToken: "must-not-leak"},
			{Status: "COMPLETED", FailRows: 1, Base: modelbase.Base{CreatedAt: now}},
			{Status: "COMPLETED", Base: modelbase.Base{CreatedAt: now}},
			{Status: "PROCESSING", FailRows: 10, Base: modelbase.Base{CreatedAt: now}},
			{Status: "FAILED", Base: modelbase.Base{CreatedAt: day.Add(-6*24*time.Hour - time.Second)}},
			{Status: "FAILED", Base: modelbase.Base{CreatedAt: now.Add(time.Second)}},
			{Status: "FAILED", Base: modelbase.Base{CreatedAt: now, DeletedAt: gorm.DeletedAt{Time: now, Valid: true}}},
		} {
			row.TaskID = fmt.Sprintf("import-%d", i)
			if err := q.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	// A corrupt cross-tenant reference must not contribute to the join aggregate.
	var platformStock invmodel.Inventory
	if err := db.WithContext(tenant.WithExactTenant(context.Background(), 0)).First(&platformStock).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(tenant.WithExactTenant(context.Background(), 99)).Create(&invmodel.InventoryTrans{InventoryID: platformStock.ID, TransType: "SHIP", CreatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	for _, tid := range []int64{0, 99, 77} {
		ctx := tenant.WithExactTenant(context.Background(), tid)
		out, err := svc.Overview(ctx, 1, all, now.UTC())
		if err != nil {
			t.Fatal(err)
		}
		if out.Activity.PutawayRecords != 2 || out.Activity.ShipmentRecords != 1 || len(out.Activity.Recent) != 3 {
			t.Fatalf("tenant %d: activity=%+v", tid, out.Activity)
		}
		if out.Activity.Recent[0].CreatedAt.Unix() != now.Unix() {
			t.Fatalf("not newest first: %+v", out.Activity.Recent)
		}
		if out.ImportIssues.Total != 2 || len(out.ImportIssues.Recent) != 2 || out.ImportIssues.Recent[0].TaskID != "import-1" {
			t.Fatalf("imports=%+v", out.ImportIssues)
		}
		bytes, _ := json.Marshal(out)
		if strings.Contains(string(bytes), "must-not-leak") || strings.Contains(string(bytes), "file_path") || strings.Contains(string(bytes), "run_token") {
			t.Fatal("private import fields exposed")
		}
		other, err := svc.Overview(ctx, 2, all, now)
		if err != nil {
			t.Fatal(err)
		}
		if other.Activity.PutawayRecords != 0 || other.Activity.ShipmentRecords != 1 || other.ImportIssues.Total != 2 {
			t.Fatalf("warehouse scope=%+v imports=%+v", other.Activity, other.ImportIssues)
		}
	}
	platform, err := svc.Overview(context.Background(), 0, all, now)
	if err != nil {
		t.Fatal(err)
	}
	if platform.Activity.PutawayRecords != 6 || platform.Activity.ShipmentRecords != 6 || platform.ImportIssues.Total != 6 {
		t.Fatalf("platform=%+v imports=%+v", platform.Activity, platform.ImportIssues)
	}
}

func TestOperationsLimitsAndDeniedQueries(t *testing.T) {
	db, svc, now := fixture(t)
	ctx := tenant.WithExactTenant(context.Background(), 99)
	q := db.WithContext(ctx)
	var stock invmodel.Inventory
	if err := q.First(&stock).Error; err != nil {
		t.Fatal(err)
	}
	for i := range 12 {
		if err := q.Create(&invmodel.InventoryTrans{InventoryID: stock.ID, TransType: "RECEIVE", CreatedAt: now.Add(-time.Duration(i) * time.Minute)}).Error; err != nil {
			t.Fatal(err)
		}
		if err := q.Create(&inboundmodel.ImportTask{TaskID: fmt.Sprint(i), Status: "FAILED", Base: modelbase.Base{CreatedAt: now.Add(-time.Duration(i) * time.Minute)}}).Error; err != nil {
			t.Fatal(err)
		}
	}
	out, err := svc.Overview(ctx, 0, all, now)
	if err != nil {
		t.Fatal(err)
	}
	if out.Activity.PutawayRecords != 12 || len(out.Activity.Recent) != 8 || out.ImportIssues.Total != 12 || len(out.ImportIssues.Recent) != 5 {
		t.Fatalf("bounded rows=%+v %+v", out.Activity, out.ImportIssues)
	}
	if err := db.Migrator().DropTable(&invmodel.InventoryTrans{}, &inboundmodel.ImportTask{}); err != nil {
		t.Fatal(err)
	}
	denied, err := svc.Overview(ctx, 0, dto.Access{Tasks: true}, now)
	if err != nil {
		t.Fatal(err)
	}
	if denied.Activity != nil || denied.ImportIssues != nil {
		t.Fatalf("denied fields=%+v", denied)
	}
}
