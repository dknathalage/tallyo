package app

import (
	"encoding/json"
	"fmt"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/participant"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/recurring"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newRecurringServer wires the recurring routes behind RequireSession + ResolveTenant, plus
// participant creation so templates can reference a valid participant FK.
func newRecurringServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "recurring.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	recH := recurring.NewHandler(recurring.NewService(conn, hub))
	pH := participant.NewHandler(participant.NewService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Post("/participants", pH.Create)
			recH.Routes(pr)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// recurringBody builds a JSON template payload for the given participant uuid.
func recurringBody(participantID string, nextDue string) string {
	return fmt.Sprintf(`{
		"participantId": %q,
		"name": "Monthly",
		"frequency": "monthly",
		"nextDue": %q,
		"lineItems": [
			{"description": "A", "quantity": 2, "unitPrice": 10, "sortOrder": 0},
			{"description": "B", "quantity": 1, "unitPrice": 5, "sortOrder": 1}
		],
		"taxRate": 10,
		"isActive": true
	}`, participantID, nextDue)
}

// createRecurring posts a template and returns its uuid.
func createRecurring(t *testing.T, c *http.Client, base, uuid string, participantID string, nextDue string) string {
	t.Helper()
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/recurring", recurringBody(participantID, nextDue))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create recurring: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode recurring: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create recurring: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestRecurringCRUD(t *testing.T) {
	srv, uuid := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, uuid, "Acme")

	id := createRecurring(t, c, srv.URL, uuid, participantID, "2026-06-01")

	// List (all).
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/recurring")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	_ = resp.Body.Close()
	if len(list) != 1 {
		t.Fatalf("list: want 1 got %d", len(list))
	}

	// List active=true.
	resp = get(t, c, srv.URL+"/api/t/"+uuid+"/recurring?active=true")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list active: want 200 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Get.
	resp = get(t, c, fmt.Sprintf("%s/api/t/%s/recurring/%s", srv.URL, uuid, id))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Update.
	upd := fmt.Sprintf(`{"participantId":%q,"name":"Renamed","frequency":"monthly","nextDue":"2026-06-01","lineItems":[{"description":"A","quantity":1,"unitPrice":10,"sortOrder":0}],"taxRate":0,"isActive":true}`, participantID)
	resp = putJSON(t, c, fmt.Sprintf("%s/api/t/%s/recurring/%s", srv.URL, uuid, id), upd)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
	var updated struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	_ = resp.Body.Close()
	if updated.Name != "Renamed" {
		t.Fatalf("update name: want Renamed got %q", updated.Name)
	}

	// Delete.
	resp = delete_(t, c, fmt.Sprintf("%s/api/t/%s/recurring/%s", srv.URL, uuid, id))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestRecurringGenerateAdvancesNextDue(t *testing.T) {
	srv, uuid := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, uuid, "Acme")
	id := createRecurring(t, c, srv.URL, uuid, participantID, "2026-06-01")

	resp := postJSON(t, c, fmt.Sprintf("%s/api/t/%s/recurring/%s/generate", srv.URL, uuid, id), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generate: want 200 got %d", resp.StatusCode)
	}
	var inv struct {
		Number    string  `json:"number"`
		Total     float64 `json:"total"`
		LineItems []struct {
			Description string `json:"description"`
		} `json:"lineItems"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&inv); err != nil {
		t.Fatalf("decode invoice: %v", err)
	}
	_ = resp.Body.Close()
	if inv.Number == "" {
		t.Fatal("generate: empty invoice number")
	}
	if len(inv.LineItems) != 2 {
		t.Fatalf("generate: want 2 line items got %d", len(inv.LineItems))
	}
	// 2*10 + 1*5 = 25, +10% tax = 27.5
	if inv.Total != 27.5 {
		t.Fatalf("generate: want total 27.5 got %v", inv.Total)
	}

	// The template's next_due must have advanced one month: 2026-06-01 -> 2026-07-01.
	resp = get(t, c, fmt.Sprintf("%s/api/t/%s/recurring/%s", srv.URL, uuid, id))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get after generate: want 200 got %d", resp.StatusCode)
	}
	var tpl struct {
		NextDue string `json:"nextDue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tpl); err != nil {
		t.Fatalf("decode template: %v", err)
	}
	_ = resp.Body.Close()
	if tpl.NextDue != "2026-07-01" {
		t.Fatalf("next_due: want 2026-07-01 got %q", tpl.NextDue)
	}
}

func TestRecurringCreateValidation(t *testing.T) {
	srv, uuid := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	pid := createParticipant(t, c, srv.URL, uuid, "Acme")

	cases := []string{
		fmt.Sprintf(`{"participantId":%q,"name":"","frequency":"monthly","nextDue":"2026-06-01"}`, pid), // empty name
		`{"participantId":"","name":"X","frequency":"monthly","nextDue":"2026-06-01"}`,                  // missing participant
		`{"name":"X","frequency":"monthly","nextDue":"2026-06-01"}`,                                     // nil participant
		fmt.Sprintf(`{"participantId":%q,"name":"X","frequency":"","nextDue":"2026-06-01"}`, pid),       // empty frequency
	}
	for i, body := range cases {
		resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/recurring", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("case %d: want 400 got %d", i, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestRecurringGetMissing404(t *testing.T) {
	srv, uuid := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/recurring/"+uuidpkg.NewString())
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing get: want 404 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestRecurringGenerateMissing404(t *testing.T) {
	srv, uuid := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/recurring/"+uuidpkg.NewString()+"/generate", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing generate: want 404 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestRecurringListUnauthenticated401(t *testing.T) {
	srv, uuid := newRecurringServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/recurring")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
