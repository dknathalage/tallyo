package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newRecurringServer wires the recurring routes behind RequireAuth, plus
// participant creation so templates can reference a valid participant FK.
func newRecurringServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "recurring.db")
	users, _, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))
	recH := NewRecurringHandler(service.NewRecurringService(conn, hub))
	pH := NewParticipantHandler(service.NewParticipantService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Post("/participants", pH.Create)
			pr.Get("/recurring", recH.List)
			pr.Post("/recurring", recH.Create)
			pr.Get("/recurring/{id}", recH.Get)
			pr.Put("/recurring/{id}", recH.Update)
			pr.Delete("/recurring/{id}", recH.Delete)
			pr.Post("/recurring/{id}/generate", recH.Generate)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// recurringBody builds a JSON template payload for the given participant.
func recurringBody(participantID int64, nextDue string) string {
	return fmt.Sprintf(`{
		"participantId": %d,
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

// createRecurring posts a template and returns its id.
func createRecurring(t *testing.T, c *http.Client, base string, participantID int64, nextDue string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/recurring", recurringBody(participantID, nextDue))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create recurring: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode recurring: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create recurring: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestRecurringCRUD(t *testing.T) {
	srv := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")

	id := createRecurring(t, c, srv.URL, participantID, "2026-06-01")

	// List (all).
	resp := get(t, c, srv.URL+"/api/recurring")
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
	resp = get(t, c, srv.URL+"/api/recurring?active=true")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list active: want 200 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Get.
	resp = get(t, c, fmt.Sprintf("%s/api/recurring/%d", srv.URL, id))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Update.
	upd := fmt.Sprintf(`{"participantId":%d,"name":"Renamed","frequency":"monthly","nextDue":"2026-06-01","lineItems":[{"description":"A","quantity":1,"unitPrice":10,"sortOrder":0}],"taxRate":0,"isActive":true}`, participantID)
	resp = putJSON(t, c, fmt.Sprintf("%s/api/recurring/%d", srv.URL, id), upd)
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
	resp = delete_(t, c, fmt.Sprintf("%s/api/recurring/%d", srv.URL, id))
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestRecurringGenerateAdvancesNextDue(t *testing.T) {
	srv := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	participantID := createParticipant(t, c, srv.URL, "Acme")
	id := createRecurring(t, c, srv.URL, participantID, "2026-06-01")

	resp := postJSON(t, c, fmt.Sprintf("%s/api/recurring/%d/generate", srv.URL, id), "")
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
	resp = get(t, c, fmt.Sprintf("%s/api/recurring/%d", srv.URL, id))
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
	srv := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	createParticipant(t, c, srv.URL, "Acme")

	cases := []string{
		`{"participantId":1,"name":"","frequency":"monthly","nextDue":"2026-06-01"}`,  // empty name
		`{"participantId":0,"name":"X","frequency":"monthly","nextDue":"2026-06-01"}`, // missing participant
		`{"name":"X","frequency":"monthly","nextDue":"2026-06-01"}`,                   // nil participant
		`{"participantId":1,"name":"X","frequency":"","nextDue":"2026-06-01"}`,        // empty frequency
	}
	for i, body := range cases {
		resp := postJSON(t, c, srv.URL+"/api/recurring", body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("case %d: want 400 got %d", i, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}

func TestRecurringGetMissing404(t *testing.T) {
	srv := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/recurring/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing get: want 404 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestRecurringGenerateMissing404(t *testing.T) {
	srv := newRecurringServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/recurring/999/generate", "")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing generate: want 404 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}

func TestRecurringListUnauthenticated401(t *testing.T) {
	srv := newRecurringServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/recurring")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()
}
