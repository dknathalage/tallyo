package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// itoa formats an int64 id for URL building.
func itoa(id int64) string { return strconv.FormatInt(id, 10) }

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

// newPayerServer wires the payer routes behind RequireAuth the same way
// production does, plus a login route so tests can authenticate.
func newPayerServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "payer.db"))
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
	pH := NewPayerHandler(service.NewPayerService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/payers", pH.List)
			pr.Post("/payers", pH.Create)
			pr.Post("/payers/bulk-delete", pH.BulkDelete)
			pr.Get("/payers/{id}", pH.Get)
			pr.Put("/payers/{id}", pH.Update)
			pr.Delete("/payers/{id}", pH.Delete)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createPayer posts a payer with the given name and returns its id.
func createPayer(t *testing.T, c *http.Client, base, name string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/payers", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create payer %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode payer: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create payer: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestPayerListEmptyReturnsArray(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/payers")
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
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPayer(t, c, srv.URL, "Acme")
	resp := get(t, c, srv.URL+"/api/payers/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestPayerGetNotFound404(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/payers/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing payer: want 404 got %d", resp.StatusCode)
	}
}

func TestPayerGetBadID400(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/payers/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestPayerCreateEmptyName400(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/payers", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestPayerUpdateOK(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPayer(t, c, srv.URL, "Acme")
	resp := putJSON(t, c, srv.URL+"/api/payers/"+itoa(id), `{"name":"Acme Corp"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestPayerUpdateMissing404(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/payers/99999", `{"name":"Nope"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestPayerDelete204(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPayer(t, c, srv.URL, "Acme")
	resp := delete_(t, c, srv.URL+"/api/payers/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestPayerBulkDelete204(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	a := createPayer(t, c, srv.URL, "A")
	b := createPayer(t, c, srv.URL, "B")
	resp := postJSON(t, c, srv.URL+"/api/payers/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestPayerListSearchFilters(t *testing.T) {
	srv := newPayerServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createPayer(t, c, srv.URL, "Acme")
	_ = createPayer(t, c, srv.URL, "Globex")

	resp := get(t, c, srv.URL+"/api/payers?search=acm")
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
	srv := newPayerServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/payers")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
