package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOptionalJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{"", " \r\n\t", "{}", `{"qty":2}`} {
		for _, length := range []int64{-1, 0, int64(len(body))} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
			c.Request.ContentLength = length
			var req struct {
				Qty int `json:"qty" binding:"omitempty,min=1,max=2000"`
			}
			if !bindOptionalJSON(c, &req) {
				t.Fatalf("valid body %q length %d rejected", body, length)
			}
			if body == `{"qty":2}` && req.Qty != 2 {
				t.Fatal("quantity was not bound")
			}
		}
	}
	for _, body := range []string{"{", `{"qty":`, `{"qty":"2"}`, `{"qty":-1}`, `{"qty":2001}`, "null", "[]", "1", `"x"`, "{}{}", "{} garbage"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/", strings.NewReader(body))
		c.Request.ContentLength = -1
		var req struct {
			Qty int `json:"qty" binding:"omitempty,min=1,max=2000"`
		}
		if bindOptionalJSON(c, &req) {
			t.Fatalf("invalid body accepted: %q", body)
		}
	}
}

// nil service makes any dispatch observable as a panic: invalid JSON must stop before side effects.
func TestScenarioHandlersRejectMalformedBodyBeforeDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := New(nil)
	handlers := map[string]gin.HandlerFunc{
		"inbound": h.runInbound, "outbound": h.runOutbound, "stocktake": h.runStocktake, "full": h.runFull,
		"inbound_drafts": h.runInboundDrafts, "outbound_drafts": h.runOutboundDrafts, "stocktake_drafts": h.runStocktakeDrafts,
		"concurrent": h.runConcurrent, "shortage": h.runConcurrentShortage, "picking": h.runConcurrentPicking, "restock": h.runRestock,
	}
	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			for _, body := range []string{"{", "null", "[]", "{}{}"} {
				for _, length := range []int64{-1, 0, int64(len(body))} {
					w := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(w)
					c.Request = httptest.NewRequest("POST", "/demo/run/"+name, strings.NewReader(body))
					c.Request.ContentLength = length
					handle(c)
					if w.Code != 400 {
						t.Fatalf("body=%q length=%d status=%d", body, length, w.Code)
					}
				}
			}
		})
	}
}
