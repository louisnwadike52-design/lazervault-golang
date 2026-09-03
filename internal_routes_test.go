package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Pins interceptInternalOnlyRoutes.
//
// The routes it blocks leaked every user's transactions (see the commit that
// added it), so "does this actually block, and does it block only what it
// should" needs to be verifiable without a production token. This wires the
// interceptor into a gin engine shaped like the real one — a wildcard
// `Any("/*path")` under `/api`, which is why the interceptor matches on
// `c.Param("path")` rather than the full URL.
func newTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	api := r.Group("/api")
	api.Use(interceptInternalOnlyRoutes())
	api.Any("/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "proxied")
	})
	return r
}

func get(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestInternalOnlyRoutes_Blocked(t *testing.T) {
	r := newTestEngine()
	blocked := []string{
		"/api/v1/transactions/by-reference/IDEM-XFER-abc123",
		"/api/v1/transactions/by-reference/SB-1",
		// Path params can contain anything; a reference with slashes or
		// encoded characters must not slip past the prefix match.
		"/api/v1/transactions/by-reference/a%2Fb",
		"/api/v1/transactions/ledger-entries",
		"/api/v1/transactions/ledger-entries?reference=X",
	}
	for _, p := range blocked {
		w := get(t, r, p)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404 (it must not reach the proxy)", p, w.Code)
		}
		// 404, never 403: an authorization error confirms the reference exists,
		// which is itself the leak.
		if w.Body.String() == "proxied" {
			t.Errorf("%s was proxied through", p)
		}
	}
}

// The blocklist must not swallow the transaction routes the app genuinely
// uses. `/api/v1/transactions` is the user's own history and is owner-scoped
// server-side; blocking it would blank the history screen.
func TestInternalOnlyRoutes_LeavesRealRoutesAlone(t *testing.T) {
	r := newTestEngine()
	allowed := []string{
		"/api/v1/transactions",
		"/api/v1/transactions?limit=20",
		"/api/v1/transactions/statistics",
		"/api/v1/transactions/dashboard",
		"/api/v1/accounts",
		"/api/v1/notifications",
		// Near-misses that share a prefix up to a point but are not the
		// blocked paths.
		"/api/v1/transactions/by-reference",
		"/api/v1/transactions/ledger-entriesx",
	}
	for _, p := range allowed {
		w := get(t, r, p)
		if w.Code != http.StatusOK {
			t.Errorf("%s = %d, want 200 — the blocklist is too broad", p, w.Code)
		}
	}
}
