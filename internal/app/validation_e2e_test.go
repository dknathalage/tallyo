package app

// End-to-end coverage of the 422 validation surface (J12): a line that fails
// validation driven through the REAL invoice and estimate HTTP handlers must
// serialize as 422 with body {error, details:[{line, field, message}, ...]} via
// WriteValidationError. The engine logic itself is unit-tested in
// internal/billing; this test pins the HTTP status + JSON shape the frontend
// editor depends on. Under the catalogue model the validation trigger is an
// unknown catalogueItemId (an unknown code is no longer an error — code is a
// free-text snapshot).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/session"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newValidationServer wires the invoice + estimate + client routes behind
// httpx.RequireAuth.
func newValidationServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "validation_e2e.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	v := newStubVerifier()
	tenants := auth.NewTenants(conn)
	invH := invoice.NewHandler(invoice.NewService(conn, session.NewService(conn, invoice.NewInvoices(conn))))
	estH := estimate.NewHandler(estimate.NewService(conn))
	pH := client.NewHandler(client.NewService(conn))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Post("/clients", pH.Create)
			invH.Routes(pr)
			pr.Post("/estimates", estH.Create)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// validationEnvelope is the 422 body shape J12 depends on.
type validationEnvelope struct {
	Error   string `json:"error"`
	Details []struct {
		Line    int    `json:"line"`
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"details"`
}

// assertValidationEnvelope decodes a 422 body and asserts the {error,details}
// shape with at least one field-level error carrying a non-empty message.
func assertValidationEnvelope(t *testing.T, resp *http.Response) {
	t.Helper()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 got %d", resp.StatusCode)
	}
	var env validationEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode 422 body: %v", err)
	}
	if env.Error == "" {
		t.Fatalf("422 body: empty error field")
	}
	if len(env.Details) == 0 {
		t.Fatalf("422 body: details must be non-empty, got %+v", env)
	}
	fe := env.Details[0]
	if fe.Field == "" || fe.Message == "" {
		t.Fatalf("422 detail must carry field+message, got %+v", fe)
	}
}

func TestInvoiceCreateUnknownCatalogueItemReturns422(t *testing.T) {
	srv, uuid := newValidationServer(t)
	c := loggedInClient(t, srv.URL)
	pid := createClient(t, c, srv.URL, uuid, "Plan Client")

	body, err := json.Marshal(map[string]any{
		"clientId": pid, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"catalogueItemId": uuidpkg.NewString(), "quantity": 1, "unitPrice": 150, "sortOrder": 0},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertValidationEnvelope(t, resp)
}

func TestEstimateCreateUnknownCatalogueItemReturns422(t *testing.T) {
	srv, uuid := newValidationServer(t)
	c := loggedInClient(t, srv.URL)
	pid := createClient(t, c, srv.URL, uuid, "Plan Client")

	body, err := json.Marshal(map[string]any{
		"clientId": pid, "issueDate": "2026-01-01", "validUntil": "2026-02-01",
		"lineItems": []map[string]any{
			{"catalogueItemId": uuidpkg.NewString(), "quantity": 1, "unitPrice": 150, "sortOrder": 0},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertValidationEnvelope(t, resp)
}
