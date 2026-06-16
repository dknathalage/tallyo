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

// newTaxRateServer wires the tax-rate routes behind RequireAuth the same way
// production does, plus a login route so tests can authenticate.
func newTaxRateServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "tax.db")
	users, _, _ := seedTenantOwner(t, conn)

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	trH := NewTaxRateHandler(service.NewTaxRateService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/tax-rates", trH.List)
			pr.Post("/tax-rates", trH.Create)
			pr.Get("/tax-rates/{id}", trH.Get)
			pr.Put("/tax-rates/{id}", trH.Update)
			pr.Delete("/tax-rates/{id}", trH.Delete)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createTaxRate posts a tax rate and returns its id.
func createTaxRate(t *testing.T, c *http.Client, base, name string, rate float64, isDefault bool) int64 {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "rate": rate, "isDefault": isDefault})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/tax-rates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tax rate %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tax rate: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create tax rate: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestTaxRateListEmptyReturnsArray(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/tax-rates")
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

func TestTaxRateCreateAndGet(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, "GST", 10, false)
	resp := get(t, c, srv.URL+"/api/tax-rates/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestTaxRateGetNotFound404(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/tax-rates/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestTaxRateGetBadID400(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/tax-rates/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateOK(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, "GST", 10, false)
	resp := putJSON(t, c, srv.URL+"/api/tax-rates/"+itoa(id), `{"name":"VAT","rate":20}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateEmptyName400(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, "GST", 10, false)
	resp := putJSON(t, c, srv.URL+"/api/tax-rates/"+itoa(id), `{"name":"","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateMissing404(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/tax-rates/99999", `{"name":"X","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestTaxRateDelete204(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, "GST", 10, false)
	resp := delete_(t, c, srv.URL+"/api/tax-rates/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestTaxRateExclusiveDefault(t *testing.T) {
	srv := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	idA := createTaxRate(t, c, srv.URL, "A", 5, true)
	idB := createTaxRate(t, c, srv.URL, "B", 10, true)

	resp := get(t, c, srv.URL+"/api/tax-rates")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var out []struct {
		ID        int64 `json:"id"`
		IsDefault bool  `json:"isDefault"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	defaults := map[int64]bool{}
	for _, r := range out {
		defaults[r.ID] = r.IsDefault
	}
	if defaults[idA] {
		t.Fatalf("A should not remain default after B set default")
	}
	if !defaults[idB] {
		t.Fatalf("B should be the only default")
	}
}

func TestTaxRateListUnauthenticated401(t *testing.T) {
	srv := newTaxRateServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/tax-rates")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
