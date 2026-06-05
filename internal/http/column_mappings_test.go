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

// newColumnMappingServer wires the column-mapping routes behind RequireAuth the
// same way production does, plus a login route so tests can authenticate.
func newColumnMappingServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "cm.db"))
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
	cmH := NewColumnMappingHandler(service.NewColumnMappingService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/column-mappings", cmH.List)
			pr.Post("/column-mappings", cmH.Create)
			pr.Get("/column-mappings/{id}", cmH.Get)
			pr.Put("/column-mappings/{id}", cmH.Update)
			pr.Delete("/column-mappings/{id}", cmH.Delete)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createMapping posts a mapping with the given name and returns its id.
func createMapping(t *testing.T, c *http.Client, base, body string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/column-mappings", body)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create mapping: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode mapping: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create mapping: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestColumnMappingListEmptyReturnsArray(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/column-mappings")
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

func TestColumnMappingCreateAndGet(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	id := createMapping(t, c, srv.URL, `{"name":"Vendor"}`)

	resp := get(t, c, srv.URL+"/api/column-mappings/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
	var out struct {
		Name       string `json:"name"`
		EntityType string `json:"entityType"`
		Mapping    string `json:"mapping"`
		FileType   string `json:"fileType"`
		HeaderRow  int64  `json:"headerRow"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Defaults applied by the repository.
	if out.Name != "Vendor" || out.EntityType != "catalog" || out.Mapping != "{}" || out.FileType != "csv" || out.HeaderRow != 1 {
		t.Fatalf("get defaults = %+v", out)
	}
}

func TestColumnMappingListByEntity(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createMapping(t, c, srv.URL, `{"name":"Cat","entityType":"catalog"}`)
	_ = createMapping(t, c, srv.URL, `{"name":"Pay","entityType":"payer"}`)

	resp := get(t, c, srv.URL+"/api/column-mappings?entityType=payer")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list by entity: want 200 got %d", resp.StatusCode)
	}
	var out []struct {
		Name       string `json:"name"`
		EntityType string `json:"entityType"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out) != 1 || out[0].EntityType != "payer" {
		t.Fatalf("list by entity = %+v, want 1 payer", out)
	}
}

func TestColumnMappingCreateEmptyName400(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/column-mappings", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestColumnMappingGetNotFound404(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/column-mappings/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing mapping: want 404 got %d", resp.StatusCode)
	}
}

func TestColumnMappingUpdateOK(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	id := createMapping(t, c, srv.URL, `{"name":"Old"}`)
	resp := putJSON(t, c, srv.URL+"/api/column-mappings/"+itoa(id), `{"name":"New","entityType":"payer"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestColumnMappingUpdateEmptyName400(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	id := createMapping(t, c, srv.URL, `{"name":"Old"}`)
	resp := putJSON(t, c, srv.URL+"/api/column-mappings/"+itoa(id), `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("update empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestColumnMappingUpdateMissing404(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/column-mappings/99999", `{"name":"New"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestColumnMappingDelete204(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := loggedInClient(t, srv.URL)
	id := createMapping(t, c, srv.URL, `{"name":"ToDelete"}`)
	resp := delete_(t, c, srv.URL+"/api/column-mappings/"+itoa(id))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}

	g := get(t, c, srv.URL+"/api/column-mappings/"+itoa(id))
	_ = g.Body.Close()
	if g.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: want 404 got %d", g.StatusCode)
	}
}

func TestColumnMappingListUnauthenticated401(t *testing.T) {
	srv := newColumnMappingServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/column-mappings")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
