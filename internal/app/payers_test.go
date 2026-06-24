package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/payer"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// delete_ issues a DELETE request with the given client.
func delete_(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("new delete req %s: %v", url, err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("do delete %s: %v", url, err)
	}
	return resp
}

// newPayerServer wires the payer routes behind RequireSession+ResolveTenant the same
// way production does, plus a login route so tests can authenticate.
func newPayerServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "payer.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	pH := payer.NewHandler(payer.NewService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pH.Routes(pr)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createPayer posts a payer with the given name and returns its uuid.
func createPayer(t *testing.T, c *http.Client, base, uuid, name string) string {
	t.Helper()
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/payers", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payer %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode payer: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create payer: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestPayerListEmptyReturnsArray(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/payers")
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

func TestPayerCreateAndGet(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPayer(t, c, srv.URL, uuid, "Acme")
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/payers/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestPayerGetNotFound404(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/payers/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing payer: want 404 got %d", resp.StatusCode)
	}
}

func TestPayerGetBadID400(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/payers/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestPayerCreateEmptyName400(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/payers", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestPayerUpdateOK(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPayer(t, c, srv.URL, uuid, "Acme")
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/payers/"+id, `{"name":"Acme Corp"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestPayerUpdateMissing404(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/payers/"+uuidpkg.NewString(), `{"name":"Nope"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestPayerDelete204(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPayer(t, c, srv.URL, uuid, "Acme")
	resp := delete_(t, c, srv.URL+"/api/t/"+uuid+"/payers/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestPayerBulkDelete204(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	a := createPayer(t, c, srv.URL, uuid, "A")
	b := createPayer(t, c, srv.URL, uuid, "B")
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/payers/bulk-delete", `{"ids":["`+a+`","`+b+`"]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestPayerListSearchFilters(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createPayer(t, c, srv.URL, uuid, "Acme")
	_ = createPayer(t, c, srv.URL, uuid, "Globex")

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/payers?search=acm")
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

func TestPayerListUnauthenticated401(t *testing.T) {
	srv, uuid := newPayerServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/payers")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
