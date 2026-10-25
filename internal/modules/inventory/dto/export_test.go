package dto

import (
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"

	"gowms/internal/modules/inventory/model"
)

func TestTransactionExportAcceptsAllLedgerTypes(t *testing.T) {
	for _, kind := range []model.TransType{model.TransOpening, model.TransReceive, model.TransAllocate, model.TransShip, model.TransRelease, model.TransAdjust, model.TransMoveIn, model.TransMoveOut, model.TransFreeze, model.TransUnfreeze, model.TransShortage, "", "INVALID"} {
		t.Run(string(kind), func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest("GET", "/inventory/trans/export?trans_type="+url.QueryEscape(string(kind)), nil)
			var query TransExportQuery
			err := ctx.ShouldBindQuery(&query)
			if kind == "INVALID" {
				if err == nil {
					t.Fatal("unknown ledger type accepted")
				}
			} else if err != nil || query.TransType != string(kind) {
				t.Fatalf("valid ledger type %s rejected: %v", kind, err)
			}
		})
	}
}
