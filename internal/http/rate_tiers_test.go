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

// newRateTierServer wires the rate-tier routes behind RequireAuth the same way
// production does, plus a login route so tests can authenticate.
func newRateTierServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "rt.db"))
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

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	rtH := NewRateTierHandler(service.NewRateTierService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/rate-tiers", rtH.List)
			pr.Post("/rate-tiers", rtH.Create)
			pr.Get("/rate-tiers/{id}", rtH.Get)
			pr.Put("/rate-tiers/{id}", rtH.Update)
			pr.Delete("/rate-tiers/{id}", rtH.Delete)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createTier posts a tier with the given name and returns its id.
func createTier(t *testing.T, c *http.Client, base, name string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/rate-tiers", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tier %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tier: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create tier: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestRateTierListEmptyReturnsArray(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/rate-tiers")
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

func TestRateTierCreateAndGet(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTier(t, c, srv.URL, "Std")

	resp := get(t, c, srv.URL+"/api/rate-tiers/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestRateTierGetNotFound404(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/rate-tiers/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing tier: want 404 got %d", resp.StatusCode)
	}
}

func TestRateTierGetBadID400(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/rate-tiers/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestRateTierUpdateOK(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTier(t, c, srv.URL, "Std")
	resp := putJSON(t, c, srv.URL+"/api/rate-tiers/"+itoa(id), `{"name":"New"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestRateTierUpdateEmptyName400(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTier(t, c, srv.URL, "Std")
	resp := putJSON(t, c, srv.URL+"/api/rate-tiers/"+itoa(id), `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("update empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestRateTierUpdateMissing404(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/rate-tiers/99999", `{"name":"New"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestRateTierDeleteThenLastTierConflict(t *testing.T) {
	srv := newRateTierServer(t)
	c := loggedInClient(t, srv.URL)
	id1 := createTier(t, c, srv.URL, "One")
	id2 := createTier(t, c, srv.URL, "Two")

	r1 := delete_(t, c, srv.URL+"/api/rate-tiers/"+itoa(id1))
	_ = r1.Body.Close()
	if r1.StatusCode != http.StatusNoContent {
		t.Fatalf("delete one: want 204 got %d", r1.StatusCode)
	}

	r2 := delete_(t, c, srv.URL+"/api/rate-tiers/"+itoa(id2))
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusConflict {
		t.Fatalf("delete last: want 409 got %d", r2.StatusCode)
	}
}

func TestRateTierListUnauthenticated401(t *testing.T) {
	srv := newRateTierServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/rate-tiers")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
