package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/planmanager"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newParticipantServer wires the participant routes behind RequireAuth plus the
// plan-manager create route so a participant can reference a plan-manager FK.
func newParticipantServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "participant.db")
	users, _, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))
	pH := NewParticipantHandler(service.NewParticipantService(conn, hub))
	pmH := planmanager.NewHandler(planmanager.NewService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Get("/participants", pH.List)
			pr.Post("/participants", pH.Create)
			pr.Post("/participants/bulk-delete", pH.BulkDelete)
			pr.Get("/participants/{id}", pH.Get)
			pr.Put("/participants/{id}", pH.Update)
			pr.Delete("/participants/{id}", pH.Delete)
			pr.Post("/plan-managers", pmH.Create)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createParticipant posts a participant with the given name and returns its id.
func createParticipant(t *testing.T, c *http.Client, base, name string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/participants", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create participant %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode participant: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create participant: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestParticipantListEmptyReturnsArray(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/participants")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	buf := make([]byte, 8)
	n, _ := resp.Body.Read(buf)
	if got := string(buf[:n]); got != "[]\n" {
		t.Fatalf("empty list: want %q got %q", "[]\n", got)
	}
}

func TestParticipantListPlanManagerName(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	pmID := createPlanManager(t, c, srv.URL, "Globex")

	body, err := json.Marshal(map[string]any{"name": "Wayne", "planManagerId": pmID, "mgmtType": "plan"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/participants", string(body))
	if resp.StatusCode != http.StatusCreated {
		_ = resp.Body.Close()
		t.Fatalf("create participant: want 201 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	lr := get(t, c, srv.URL+"/api/participants")
	defer func() { _ = lr.Body.Close() }()
	var out []struct {
		Name            string `json:"name"`
		PlanManagerName string `json:"planManagerName"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 participant got %d", len(out))
	}
	if out[0].PlanManagerName != "Globex" {
		t.Fatalf("planManagerName: want Globex got %q", out[0].PlanManagerName)
	}
}

func TestParticipantCreateWithFieldsAndGet(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	pmID := createPlanManager(t, c, srv.URL, "Acme")
	body, err := json.Marshal(map[string]any{
		"name": "Stark", "ndisNumber": "430000001", "mgmtType": "plan",
		"planManagerId": pmID, "email": "s@x.com", "phone": "123", "address": "1 St",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/participants", string(body))
	if resp.StatusCode != http.StatusCreated {
		_ = resp.Body.Close()
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		_ = resp.Body.Close()
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()

	gr := get(t, c, srv.URL+"/api/participants/"+itoa(out.ID))
	defer func() { _ = gr.Body.Close() }()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", gr.StatusCode)
	}
	var p struct {
		NDISNumber string `json:"ndisNumber"`
	}
	if err := json.NewDecoder(gr.Body).Decode(&p); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if p.NDISNumber != "430000001" {
		t.Fatalf("ndisNumber: want 430000001 got %q", p.NDISNumber)
	}
}

func TestParticipantGetNotFound404(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/participants/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestParticipantCreateEmptyName400(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/participants", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestParticipantUpdateOK(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	id := createParticipant(t, c, srv.URL, "Stark")
	resp := putJSON(t, c, srv.URL+"/api/participants/"+itoa(id), `{"name":"Stark Industries"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestParticipantUpdateMissing404(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/participants/99999", `{"name":"Nope"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestParticipantUpdateEmptyName400(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	id := createParticipant(t, c, srv.URL, "Stark")
	resp := putJSON(t, c, srv.URL+"/api/participants/"+itoa(id), `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("update empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestParticipantDelete204(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	id := createParticipant(t, c, srv.URL, "Stark")
	resp := delete_(t, c, srv.URL+"/api/participants/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestParticipantBulkDelete204(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	a := createParticipant(t, c, srv.URL, "A")
	b := createParticipant(t, c, srv.URL, "B")
	resp := postJSON(t, c, srv.URL+"/api/participants/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestParticipantListSearchFilters(t *testing.T) {
	srv := newParticipantServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createParticipant(t, c, srv.URL, "Acme")
	_ = createParticipant(t, c, srv.URL, "Globex")

	resp := get(t, c, srv.URL+"/api/participants?search=acm")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("search: want 200 got %d", resp.StatusCode)
	}
	var out []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if len(out) != 1 || out[0].Name != "Acme" {
		t.Fatalf("search=acm: want [Acme] got %+v", out)
	}
}

func TestParticipantListUnauthenticated401(t *testing.T) {
	srv := newParticipantServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/participants")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
