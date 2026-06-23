package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/customitem"
	"github.com/dknathalage/tallyo/internal/pricelist"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newCustomItemServer wires the per-tenant custom-item routes and the read-only
// price-list routes behind httpx.RequireAuth.
func newCustomItemServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "custom_items.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)
	ciH := customitem.NewHandler(customitem.NewService(conn, hub))
	scH := pricelist.NewHandler(pricelist.NewService(conn), pricelist.NewImportService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			ciH.Routes(pr)
			pr.Get("/price-list/versions", scH.ListVersions)
			pr.Get("/price-list/versions/{id}/items", scH.ListItems)
			pr.Get("/price-list/items/{itemId}/prices", scH.ListPrices)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createCustomItem posts a custom item and returns its uuid.
func createCustomItem(t *testing.T, c *http.Client, base, uuid, name string) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "rate": 100.0, "unit": "hour"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/custom-items", string(body))
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

func TestCustomItemListEmptyReturnsArray(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/custom-items")
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

func TestCustomItemCreateAndGet(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCustomItem(t, c, srv.URL, uuid, "Widget")
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestCustomItemGetNotFound404(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCustomItemGetBadID400(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestCustomItemCreateEmptyName400(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/custom-items", `{"name":"","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestCustomItemUpdateOK(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCustomItem(t, c, srv.URL, uuid, "Widget")
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/"+id, `{"name":"Gadget","rate":200}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestCustomItemUpdateMissing404(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/"+uuidpkg.NewString(), `{"name":"X","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCustomItemDelete204(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCustomItem(t, c, srv.URL, uuid, "Widget")
	resp := delete_(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCustomItemBulkDelete204(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	a := createCustomItem(t, c, srv.URL, uuid, "A")
	b := createCustomItem(t, c, srv.URL, uuid, "B")
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/custom-items/bulk-delete", `{"ids":["`+a+`","`+b+`"]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCustomItemListSearchFilters(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createCustomItem(t, c, srv.URL, uuid, "Widget")
	_ = createCustomItem(t, c, srv.URL, uuid, "Gizmo")

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/custom-items?search=widg")
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

func TestCustomItemListUnauthenticated401(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/custom-items")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

// TestPriceListVersionsEmptyReturnsArray smoke-tests the read-only price-list
// route: with no price list ingested, it returns [].
func TestPriceListVersionsEmptyReturnsArray(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/price-list/versions")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("versions: want 200 got %d", resp.StatusCode)
	}
	buf := make([]byte, 8)
	n, _ := resp.Body.Read(buf)
	if got := string(buf[:n]); got != "[]\n" {
		t.Fatalf("empty versions: want %q got %q", "[]\n", got)
	}
}

func TestPriceListUnauthenticated401(t *testing.T) {
	srv, uuid := newCustomItemServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/price-list/versions")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon versions: want 401 got %d", resp.StatusCode)
	}
}
