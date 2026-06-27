package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/subscription"
	"github.com/go-chi/chi/v5"
)

// TestSubscriptionGate verifies the live wiring: with billing on and a tenant in
// status "none" (default), write methods on gated routes return 402 while reads
// and the ungated billing routes still work; once the tenant is entitled, writes
// pass. ResolveTenant computes entitlement from the DB-backed status here.
func TestSubscriptionGate(t *testing.T) {
	conn := openMigratedDB(t, "g.db")
	users, tenantID, _, tenantUUID := seedTenantOwner(t, conn)
	tenants := auth.NewTenants(conn)
	v := newStubVerifier()

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants, true)) // billing ON
			pr.Get("/probe", probe200)
			pr.Post("/billing/checkout", probe200) // ungated billing stand-in
			pr.Group(func(gr chi.Router) {
				gr.Use(httpx.RequireSubscription)
				gr.Post("/invoices", probe200)
			})
		})
	})

	do := func(method, path string) int {
		req, _ := http.NewRequest(method, "/api/t/"+tenantUUID+path, nil)
		req.Header.Set("Authorization", "Bearer "+ownerToken)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	// status "none" → not entitled.
	if c := do(http.MethodPost, "/invoices"); c != http.StatusPaymentRequired {
		t.Errorf("lapsed POST /invoices = %d, want 402", c)
	}
	if c := do(http.MethodGet, "/probe"); c != http.StatusOK {
		t.Errorf("lapsed GET /probe = %d, want 200", c)
	}
	if c := do(http.MethodPost, "/billing/checkout"); c != http.StatusOK {
		t.Errorf("lapsed POST /billing/checkout = %d, want 200 (ungated)", c)
	}

	// Subscribe → entitled → writes pass.
	if _, err := subscription.NewStore(conn).Apply(context.Background(), subscription.Update{
		TenantID: tenantID, StripeCustomerID: "cus_1", Status: subscription.StatusActive,
		SyncedAt: "2026-06-27T10:00:00Z",
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if c := do(http.MethodPost, "/invoices"); c != http.StatusOK {
		t.Errorf("entitled POST /invoices = %d, want 200", c)
	}
}
