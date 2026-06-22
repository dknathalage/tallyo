package app

// End-to-end coverage of the 422 validation surface (spec §10 / J12): an
// over-cap support line driven through the REAL invoice and estimate HTTP
// handlers must serialize as 422 with body
// {error, details:[{line, field, message}, ...]} via WriteValidationError.
// The engine logic itself is unit-tested in internal/service; this test pins
// the HTTP status + JSON shape the frontend editor depends on.

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/db/gen"
	"github.com/dknathalage/tallyo/internal/estimate"
	"github.com/dknathalage/tallyo/internal/invoice"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/shift"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// newValidationServer wires the invoice + estimate + participant routes behind
// httpx.RequireAuth and returns both the server and the underlying conn so the test
// can seed a catalogue version directly.
func newValidationServer(t *testing.T) (*httptest.Server, *sql.DB, string) {
	t.Helper()
	conn := openMigratedDB(t, "validation_e2e.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	invH := invoice.NewHandler(invoice.NewService(conn, conn, hub, shift.NewService(conn, conn, hub, invoice.NewInvoices(conn))))
	estH := estimate.NewHandler(estimate.NewService(conn, conn, hub))
	pH := participant.NewHandler(participant.NewService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Post("/participants", pH.Create)
			invH.Routes(pr)
			pr.Post("/estimates", estH.Create)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, conn, tenantUUID
}

// seedNationalCap inserts a one-item catalogue version priced at the given
// national cap, valid across the given window.
func seedNationalCap(t *testing.T, conn *sql.DB, from, to, code string, cap float64) {
	t.Helper()
	ctx := context.Background()
	q := gen.New(conn)
	now := time.Now().UTC().Format(time.RFC3339)
	v, err := q.CreateCatalogVersion(ctx, gen.CreateCatalogVersionParams{
		Uuid: uuid.NewString(), Label: "v1", EffectiveFrom: from,
		EffectiveTo: sql.NullString{String: to, Valid: true}, CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateCatalogVersion: %v", err)
	}
	si, err := q.CreateSupportItem(ctx, gen.CreateSupportItemParams{
		Uuid: uuid.NewString(), CatalogVersionID: v.ID, Code: code, Name: "Item " + code, Taxable: 0,
	})
	if err != nil {
		t.Fatalf("CreateSupportItem: %v", err)
	}
	if _, err := q.CreateSupportItemPrice(ctx, gen.CreateSupportItemPriceParams{
		SupportItemID: si.ID, Zone: "national", PriceCap: sql.NullFloat64{Float64: cap, Valid: true},
	}); err != nil {
		t.Fatalf("CreateSupportItemPrice: %v", err)
	}
}

// createParticipantWithPlan posts a participant carrying an explicit plan window
// and returns its uuid.
func createParticipantWithPlan(t *testing.T, c *http.Client, base, uuid, planStart, planEnd string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"name": "Plan Participant", "planStart": planStart, "planEnd": planEnd,
	})
	if err != nil {
		t.Fatalf("marshal participant: %v", err)
	}
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/participants", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create participant: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode participant: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create participant: want non-empty uuid got %q", out.ID)
	}
	return out.ID
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

func TestInvoiceCreateOverCapReturns422(t *testing.T) {
	srv, conn, uuid := newValidationServer(t)
	c := loggedInClient(t, srv.URL)
	seedNationalCap(t, conn, "2025-07-01", "2026-06-30", "01_011", 100)
	pid := createParticipantWithPlan(t, c, srv.URL, uuid, "2025-07-01", "2026-06-30")

	body, err := json.Marshal(map[string]any{
		"participantId": pid, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			{"code": "01_011", "serviceDate": "2026-01-15", "quantity": 1, "unitPrice": 150, "sortOrder": 0},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertValidationEnvelope(t, resp)
}

func TestInvoiceCreateOutOfPlanReturns422(t *testing.T) {
	srv, conn, uuid := newValidationServer(t)
	c := loggedInClient(t, srv.URL)
	// Catalogue window is wide; the participant plan ends 2025-12-31.
	seedNationalCap(t, conn, "2025-01-01", "2026-12-31", "01_011", 100)
	pid := createParticipantWithPlan(t, c, srv.URL, uuid, "2025-07-01", "2025-12-31")

	body, err := json.Marshal(map[string]any{
		"participantId": pid, "issueDate": "2026-01-01", "dueDate": "2026-02-01",
		"lineItems": []map[string]any{
			// Service date is after the plan end (but in a valid catalogue window).
			{"code": "01_011", "serviceDate": "2026-02-01", "quantity": 1, "unitPrice": 50, "sortOrder": 0},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/invoices", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertValidationEnvelope(t, resp)
}

func TestEstimateCreateOverCapReturns422(t *testing.T) {
	srv, conn, uuid := newValidationServer(t)
	c := loggedInClient(t, srv.URL)
	seedNationalCap(t, conn, "2025-07-01", "2026-06-30", "01_011", 100)
	pid := createParticipantWithPlan(t, c, srv.URL, uuid, "2025-07-01", "2026-06-30")

	body, err := json.Marshal(map[string]any{
		"participantId": pid, "issueDate": "2026-01-01", "validUntil": "2026-02-01",
		"lineItems": []map[string]any{
			{"code": "01_011", "serviceDate": "2026-01-15", "quantity": 1, "unitPrice": 150, "sortOrder": 0},
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/estimates", string(body))
	defer func() { _ = resp.Body.Close() }()
	assertValidationEnvelope(t, resp)
}
