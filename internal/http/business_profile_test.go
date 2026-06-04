package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newBusinessProfileServer wires the business-profile routes behind RequireAuth
// the same way production does, plus a login route so tests can authenticate.
func newBusinessProfileServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "bp.db"))
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
	bpH := NewBusinessProfileHandler(service.NewBusinessProfileService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/business-profile", bpH.Get)
			pr.Put("/business-profile", bpH.Put)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// putJSON sends a PUT with a JSON body using the given client.
func putJSON(t *testing.T, c *http.Client, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest("PUT", url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new put req %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do put %s: %v", url, err)
	}
	return resp
}

func TestBusinessProfileGetEmptyReturnsNull(t *testing.T) {
	srv := newBusinessProfileServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/business-profile")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get empty: want 200 got %d", resp.StatusCode)
	}
	// svc.Get returns nil on empty DB; WriteJSON encodes that as JSON null.
	buf := make([]byte, 16)
	n, _ := resp.Body.Read(buf)
	if got := string(buf[:n]); got != "null\n" {
		t.Fatalf("get empty: want body %q got %q", "null\n", got)
	}
}

func TestBusinessProfilePutThenGetRoundTrip(t *testing.T) {
	srv := newBusinessProfileServer(t)
	c := loggedInClient(t, srv.URL)

	put := putJSON(t, c, srv.URL+"/api/business-profile", `{"name":"Acme","email":"a@b.com"}`)
	_ = put.Body.Close()
	if put.StatusCode != http.StatusOK {
		t.Fatalf("put: want 200 got %d", put.StatusCode)
	}

	resp := get(t, c, srv.URL+"/api/business-profile")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get after put: want 200 got %d", resp.StatusCode)
	}
	var out struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if out.Name != "Acme" || out.Email != "a@b.com" {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
}

func TestBusinessProfilePutEmptyName400(t *testing.T) {
	srv := newBusinessProfileServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/business-profile", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestBusinessProfileGetUnauthenticated401(t *testing.T) {
	srv := newBusinessProfileServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/business-profile")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon get: want 401 got %d", resp.StatusCode)
	}
}
