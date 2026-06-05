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

// newCatalogServer wires the catalog routes (including tier-rate sub-routes)
// behind RequireAuth, plus a rate-tier create route for sub-route fixtures.
func newCatalogServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "catalog.db"))
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
	catH := NewCatalogHandler(service.NewCatalogService(conn, hub))
	rtH := NewRateTierHandler(service.NewRateTierService(conn, hub))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/catalog", catH.List)
			pr.Post("/catalog", catH.Create)
			pr.Get("/catalog/categories", catH.Categories)
			pr.Post("/catalog/bulk-delete", catH.BulkDelete)
			pr.Get("/catalog/{id}", catH.Get)
			pr.Put("/catalog/{id}", catH.Update)
			pr.Delete("/catalog/{id}", catH.Delete)
			pr.Get("/catalog/{id}/rates", catH.GetRates)
			pr.Put("/catalog/{id}/rates/{tierId}", catH.SetRate)
			pr.Post("/rate-tiers", rtH.Create)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createCatalogItem posts a catalog item and returns its id.
func createCatalogItem(t *testing.T, c *http.Client, base, name, category string) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "rate": 100.0, "category": category})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/catalog", string(body))
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

func TestCatalogListEmptyReturnsArray(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/catalog")
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

func TestCatalogCreateAndGet(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	resp := get(t, c, srv.URL+"/api/catalog/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestCatalogGetNotFound404(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/catalog/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCatalogGetBadID400(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/catalog/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestCatalogCreateEmptyName400(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/catalog", `{"name":"","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestCatalogUpdateOK(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	resp := putJSON(t, c, srv.URL+"/api/catalog/"+itoa(id), `{"name":"Gadget","rate":200}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestCatalogUpdateMissing404(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/catalog/99999", `{"name":"X","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestCatalogDelete204(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	id := createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	resp := delete_(t, c, srv.URL+"/api/catalog/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCatalogCategoriesArray(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	_ = createCatalogItem(t, c, srv.URL, "Manual", "Docs")

	resp := get(t, c, srv.URL+"/api/catalog/categories")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("categories: want 200 got %d", resp.StatusCode)
	}
	var out []string
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode categories: %v", err)
	}
	if len(out) < 2 {
		t.Fatalf("categories: want >=2 got %v", out)
	}
}

func TestCatalogBulkDelete204(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	a := createCatalogItem(t, c, srv.URL, "A", "X")
	b := createCatalogItem(t, c, srv.URL, "B", "Y")
	resp := postJSON(t, c, srv.URL+"/api/catalog/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestCatalogListSearchFilters(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	_ = createCatalogItem(t, c, srv.URL, "Gizmo", "Hardware")

	resp := get(t, c, srv.URL+"/api/catalog?search=widg")
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

func TestCatalogTierRatesSetAndGet(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	itemID := createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	tierID := createTier(t, c, srv.URL, "Gold")

	pr := putJSON(t, c, srv.URL+"/api/catalog/"+itoa(itemID)+"/rates/"+itoa(tierID), `{"rate":7.5}`)
	if pr.StatusCode != http.StatusOK {
		_ = pr.Body.Close()
		t.Fatalf("set rate: want 200 got %d", pr.StatusCode)
	}
	_ = pr.Body.Close()

	gr := get(t, c, srv.URL+"/api/catalog/"+itoa(itemID)+"/rates")
	defer func() { _ = gr.Body.Close() }()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get rates: want 200 got %d", gr.StatusCode)
	}
	var out []struct {
		RateTierID int64   `json:"rateTierId"`
		Rate       float64 `json:"rate"`
	}
	if err := json.NewDecoder(gr.Body).Decode(&out); err != nil {
		t.Fatalf("decode rates: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("rates: want 1 got %d", len(out))
	}
	if out[0].RateTierID != tierID || out[0].Rate != 7.5 {
		t.Fatalf("rates: want {tier:%d rate:7.5} got %+v", tierID, out[0])
	}
}

func TestCatalogSetRateBadTierID400(t *testing.T) {
	srv := newCatalogServer(t)
	c := loggedInClient(t, srv.URL)
	itemID := createCatalogItem(t, c, srv.URL, "Widget", "Hardware")
	resp := putJSON(t, c, srv.URL+"/api/catalog/"+itoa(itemID)+"/rates/abc", `{"rate":7.5}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad tier id: want 400 got %d", resp.StatusCode)
	}
}

func TestCatalogListUnauthenticated401(t *testing.T) {
	srv := newCatalogServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/catalog")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
