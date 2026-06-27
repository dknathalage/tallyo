package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/catalogue"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newCatalogueServer wires the per-tenant catalogue routes behind httpx.RequireAuth.
func newCatalogueServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "catalogue.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	v := newStubVerifier()
	tenants := auth.NewTenants(conn)
	catH := catalogue.NewHandler(catalogue.NewService(conn))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants))
			catH.Routes(pr)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createCatalogueItem posts a catalogue item and returns its uuid.
func createCatalogueItem(t *testing.T, c *http.Client, base, uuid, name string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "unitPrice": 100.0, "unit": "hour"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/catalogue", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create item %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create item: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestCatalogueListEmptyReturnsArray(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/catalogue")
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

func TestCatalogueCreateAndGet(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogueItem(t, c, srv.URL, uuid, "Widget")
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestCatalogueGetNotFound404(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCatalogueGetBadID400(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestCatalogueCreateEmptyName422(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/catalogue", `{"name":"","unitPrice":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: want 422 got %d", resp.StatusCode)
	}
}

func TestCatalogueUpdateOK(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogueItem(t, c, srv.URL, uuid, "Widget")
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/"+id, `{"name":"Gadget","unitPrice":200}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestCatalogueUpdateMissing404(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/"+uuidpkg.NewString(), `{"name":"X","unitPrice":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCatalogueDelete204(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogueItem(t, c, srv.URL, uuid, "Widget")
	resp := delete_(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCatalogueBulkDelete204(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	a := createCatalogueItem(t, c, srv.URL, uuid, "A")
	b := createCatalogueItem(t, c, srv.URL, uuid, "B")
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/bulk-delete", `{"ids":["`+a+`","`+b+`"]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCatalogueUpdateMalformedJSON400(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogueItem(t, c, srv.URL, uuid, "Widget")
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/catalogue/"+id, "{")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed JSON update: want 400 got %d", resp.StatusCode)
	}
}

func TestCatalogueListSearchFilters(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createCatalogueItem(t, c, srv.URL, uuid, "Widget")
	_ = createCatalogueItem(t, c, srv.URL, uuid, "Gizmo")

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/catalogue?search=widg")
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
	if len(out) != 1 || out[0].Name != "Widget" {
		t.Fatalf("search=widg: want [Widget] got %+v", out)
	}
}

func TestCatalogueListUnauthenticated401(t *testing.T) {
	srv, uuid := newCatalogueServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/catalogue")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
