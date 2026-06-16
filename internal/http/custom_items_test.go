package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/realtime"
	"github.com/dknathalage/tallyo/internal/service"
	"github.com/go-chi/chi/v5"
)

// newCustomItemServer wires the per-tenant custom-item routes and the read-only
// global support-catalog routes behind RequireAuth.
func newCustomItemServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "custom_items.db")
	users, _, _ := seedTenantOwner(t, conn)

	hub := realtime.NewHub()
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, auth.NewTenants(conn))
	ciH := NewCustomItemHandler(service.NewCustomItemService(conn, hub))
	scH := NewSupportCatalogHandler(service.NewSupportCatalogService(conn), service.NewCatalogIngestService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users, auth.NewTenants(conn)))
			pr.Get("/custom-items", ciH.List)
			pr.Post("/custom-items", ciH.Create)
			pr.Post("/custom-items/bulk-delete", ciH.BulkDelete)
			pr.Get("/custom-items/{id}", ciH.Get)
			pr.Put("/custom-items/{id}", ciH.Update)
			pr.Delete("/custom-items/{id}", ciH.Delete)
			pr.Get("/support-catalog/versions", scH.ListVersions)
			pr.Get("/support-catalog/versions/{id}/items", scH.ListItems)
			pr.Get("/support-catalog/items/{itemId}/prices", scH.ListPrices)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createCustomItem posts a custom item and returns its id.
func createCustomItem(t *testing.T, c *http.Client, base, name string) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "rate": 100.0, "unit": "hour"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/custom-items", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create item %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode item: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create item: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestCustomItemListEmptyReturnsArray(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/custom-items")
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
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCustomItem(t, c, srv.URL, "Widget")
	resp := get(t, c, srv.URL+"/api/custom-items/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestCustomItemGetNotFound404(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/custom-items/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCustomItemGetBadID400(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/custom-items/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestCustomItemCreateEmptyName400(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/custom-items", `{"name":"","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestCustomItemUpdateOK(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCustomItem(t, c, srv.URL, "Widget")
	resp := putJSON(t, c, srv.URL+"/api/custom-items/"+itoa(id), `{"name":"Gadget","rate":200}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestCustomItemUpdateMissing404(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/custom-items/99999", `{"name":"X","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCustomItemDelete204(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCustomItem(t, c, srv.URL, "Widget")
	resp := delete_(t, c, srv.URL+"/api/custom-items/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCustomItemBulkDelete204(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	a := createCustomItem(t, c, srv.URL, "A")
	b := createCustomItem(t, c, srv.URL, "B")
	resp := postJSON(t, c, srv.URL+"/api/custom-items/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCustomItemListSearchFilters(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createCustomItem(t, c, srv.URL, "Widget")
	_ = createCustomItem(t, c, srv.URL, "Gizmo")

	resp := get(t, c, srv.URL+"/api/custom-items?search=widg")
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
	srv := newCustomItemServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/custom-items")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

// TestSupportCatalogVersionsEmptyReturnsArray smoke-tests the read-only global
// NDIS support-catalogue route: with no catalogue ingested (J7), it returns [].
func TestSupportCatalogVersionsEmptyReturnsArray(t *testing.T) {
	srv := newCustomItemServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/support-catalog/versions")
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

func TestSupportCatalogUnauthenticated401(t *testing.T) {
	srv := newCustomItemServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/support-catalog/versions")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon versions: want 401 got %d", resp.StatusCode)
	}
}
