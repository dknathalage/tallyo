package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newClientServer wires the client routes behind RequireAuth plus rate-tier and
// payer routes so FK references can be created for join-name assertions.
func newClientServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "client.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	users := auth.NewUsers(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if _, err := users.Create(t.Context(), "o@x.com", hash, "owner"); err != nil {
		t.Fatalf("Create owner: %v", err)
	}

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	cH := NewClientHandler(service.NewClientService(conn, hub))
	rtH := NewRateTierHandler(service.NewRateTierService(conn, hub))
	pH := NewPayerHandler(service.NewPayerService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/clients", cH.List)
			pr.Post("/clients", cH.Create)
			pr.Post("/clients/bulk-delete", cH.BulkDelete)
			pr.Get("/clients/{id}", cH.Get)
			pr.Put("/clients/{id}", cH.Update)
			pr.Delete("/clients/{id}", cH.Delete)
			pr.Post("/rate-tiers", rtH.Create)
			pr.Post("/payers", pH.Create)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createClient posts a client with the given name and returns its id.
func createClient(t *testing.T, c *http.Client, base, name string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/clients", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create client %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode client: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create client: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestClientListEmptyReturnsArray(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/clients")
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

func TestClientListJoinNames(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	tierID := createTier(t, c, srv.URL, "Gold")
	payerID := createPayer(t, c, srv.URL, "Globex")

	body, err := json.Marshal(map[string]any{"name": "Wayne", "pricingTierId": tierID, "payerId": payerID})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/clients", string(body))
	if resp.StatusCode != http.StatusCreated {
		_ = resp.Body.Close()
		t.Fatalf("create client: want 201 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	lr := get(t, c, srv.URL+"/api/clients")
	defer func() { _ = lr.Body.Close() }()
	var out []struct {
		Name            string `json:"name"`
		PricingTierName string `json:"pricingTierName"`
		PayerName       string `json:"payerName"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 client got %d", len(out))
	}
	if out[0].PricingTierName != "Gold" {
		t.Fatalf("pricingTierName: want Gold got %q", out[0].PricingTierName)
	}
	if out[0].PayerName != "Globex" {
		t.Fatalf("payerName: want Globex got %q", out[0].PayerName)
	}
}

func TestClientCreateWithFKsAndGet(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	tierID := createTier(t, c, srv.URL, "Silver")
	payerID := createPayer(t, c, srv.URL, "Acme")
	body, err := json.Marshal(map[string]any{"name": "Stark", "pricingTierId": tierID, "payerId": payerID})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/clients", string(body))
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

	gr := get(t, c, srv.URL+"/api/clients/"+itoa(out.ID))
	defer func() { _ = gr.Body.Close() }()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", gr.StatusCode)
	}
}

func TestClientGetNotFound404(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/clients/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestClientCreateEmptyName400(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/clients", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestClientUpdateOK(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	id := createClient(t, c, srv.URL, "Stark")
	resp := putJSON(t, c, srv.URL+"/api/clients/"+itoa(id), `{"name":"Stark Industries"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestClientUpdateMissing404(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/clients/99999", `{"name":"Nope"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestClientUpdateEmptyName400(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	id := createClient(t, c, srv.URL, "Stark")
	resp := putJSON(t, c, srv.URL+"/api/clients/"+itoa(id), `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("update empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestClientDelete204(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	id := createClient(t, c, srv.URL, "Stark")
	resp := delete_(t, c, srv.URL+"/api/clients/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestClientBulkDelete204(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	a := createClient(t, c, srv.URL, "A")
	b := createClient(t, c, srv.URL, "B")
	resp := postJSON(t, c, srv.URL+"/api/clients/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestClientListSearchFilters(t *testing.T) {
	srv := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createClient(t, c, srv.URL, "Acme")
	_ = createClient(t, c, srv.URL, "Globex")

	resp := get(t, c, srv.URL+"/api/clients?search=acm")
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

func TestClientListUnauthenticated401(t *testing.T) {
	srv := newClientServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/clients")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
