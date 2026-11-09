package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	basicmodel "gowms/internal/modules/basic/model"
	indto "gowms/internal/modules/inbound/dto"
	sysmodel "gowms/internal/modules/system/model"
	"gowms/internal/pkg/config"
	"gowms/internal/pkg/errcode"
	"gowms/internal/pkg/jwt"
	"gowms/internal/pkg/modelbase"
	"gowms/internal/pkg/tenant"
)

func TestInboundDraftHTTPRejectsStaleEditor(t *testing.T) {
	s := newBusinessTenantStack(t)
	ctx := tenant.WithExactTenant(context.Background(), 11)
	for _, row := range []any{
		&basicmodel.Warehouse{Base: modelbase.Base{ID: 810}, Code: "EDIT-WH", Name: "warehouse", Status: 1},
		&basicmodel.SKU{Base: modelbase.Base{ID: 811}, Code: "EDIT-SKU", Name: "sku", Status: 1},
		&sysmodel.SysUser{Base: modelbase.Base{ID: 812}, Username: "editor", PasswordHash: "unused", Status: 1},
		&sysmodel.SysRole{Base: modelbase.Base{ID: 813}, Name: "editor", Perms: "wms:inbound:view,wms:inbound:create"},
		&sysmodel.SysUserRole{UserID: 812, RoleID: 813},
	} {
		requireBusinessError(t, s.db.WithContext(ctx).Create(row).Error, nil)
	}
	order, err := s.in.Create(ctx, &indto.CreateOrderReq{WarehouseID: 810, Details: []indto.OrderDetailItem{{SKUID: 811, ExpectedQty: 2}}}, "editor")
	requireBusinessError(t, err, nil)
	cfg := &config.Config{JWT: config.JWTConfig{Secret: strings.Repeat("e", 32)}}
	a := New(cfg, s.db, nil, nil)
	router, err := a.NewRouter()
	requireBusinessError(t, err, nil)
	token, err := jwt.Generate(cfg.JWT.Secret, time.Hour, 812, "editor", 1, 11)
	requireBusinessError(t, err, nil)
	request := func(method, body string, wantHTTP, wantCode int) json.RawMessage {
		t.Helper()
		req := httptest.NewRequest(method, fmt.Sprintf("/api/v1/inbound/orders/%d", order.ID), strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var response struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		requireBusinessError(t, json.Unmarshal(w.Body.Bytes(), &response), nil)
		if w.Code != wantHTTP || response.Code != wantCode {
			t.Fatalf("%s returned %d %s; want %d/%d", method, w.Code, w.Body.String(), wantHTTP, wantCode)
		}
		return response.Data
	}
	body := func(version, qty int, remark string) string {
		return fmt.Sprintf(`{"expected_version":%d,"warehouse_id":"810","remark":%q,"details":[{"sku_id":"811","expected_qty":%d}]}`, version, remark, qty)
	}
	// Both editors opened quantity 2. A saves 5; B only edits the remark and
	// must not restore its stale quantity 2.
	request("PUT", body(order.Version, 5, "editor A"), 200, 0)
	request("PUT", body(order.Version, 2, "editor B"), 409, 40902)
	current, err := s.in.Get(ctx, order.ID)
	requireBusinessError(t, err, nil)
	if current.Order.ExpectedQty != 5 || current.Order.Remark != "editor A" || len(current.Details) != 1 || current.Details[0].ExpectedQty != 5 {
		t.Fatal("stale editor overwrote header/details")
	}
	for _, badVersion := range []int{0, -1} {
		request("PUT", body(badVersion, 3, "invalid"), 400, 400)
	}
	request("PUT", `{"warehouse_id":"810","details":[{"sku_id":"811","expected_qty":3}]}`, 400, 400)
	var detail struct {
		Order struct {
			Version int `json:"version"`
		} `json:"order"`
	}
	requireBusinessError(t, json.Unmarshal(request("GET", "", 200, 0), &detail), nil)
	if detail.Order.Version != current.Order.Version {
		t.Fatal("HTTP detail lost editing revision")
	}
	request("PUT", body(detail.Order.Version, 3, "reviewed B"), 200, 0)
	latest, err := s.in.Get(ctx, order.ID)
	requireBusinessError(t, err, nil)
	if latest.Order.ExpectedQty != 3 || latest.Order.Version != detail.Order.Version+1 {
		t.Fatal("fresh editor failed to save")
	}
	// Competing requests carrying the same editor revision have one winner.
	start := make(chan struct{})
	done := make(chan error, 2)
	for _, qty := range []int{7, 9} {
		go func() {
			<-start
			done <- s.in.Update(ctx, order.ID, &indto.UpdateOrderReq{
				CreateOrderReq:  indto.CreateOrderReq{WarehouseID: 810, Details: []indto.OrderDetailItem{{SKUID: 811, ExpectedQty: qty}}},
				ExpectedVersion: latest.Order.Version,
			})
		}()
	}
	close(start)
	first, second := <-done, <-done
	firstWon := first == nil && errors.Is(second, errcode.StaleEdit)
	secondWon := second == nil && errors.Is(first, errcode.StaleEdit)
	if !firstWon && !secondWon {
		t.Fatalf("competing editors: %v / %v", first, second)
	}
	final, err := s.in.Get(ctx, order.ID)
	requireBusinessError(t, err, nil)
	if final.Order.Version != latest.Order.Version+1 || len(final.Details) != 1 || final.Order.ExpectedQty != final.Details[0].ExpectedQty {
		t.Fatal("competing edit lost aggregate consistency")
	}
}
