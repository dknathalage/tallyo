package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/taxrate"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newTaxRateServer wires the tax-rate routes behind RequireSession + ResolveTenant the same way
// production does, plus a login route so tests can authenticate.
func newTaxRateServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "tax.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	v := newStubVerifier()
	tenants := auth.NewTenants(conn)
	trH := taxrate.NewHandler(taxrate.NewService(conn))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants))
			trH.Routes(pr)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createTaxRate posts a tax rate and returns its uuid.
func createTaxRate(t *testing.T, c *http.Client, base, uuid, name string, rate float64, isDefault bool) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"name": name, "rate": rate, "isDefault": isDefault})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/tax-rates", string(body))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tax rate %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode tax rate: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create tax rate: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestTaxRateListEmptyReturnsArray(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates")
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
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, uuid, "GST", 10, false)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestTaxRateGetNotFound404(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestTaxRateGetBadID400(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateOK(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, uuid, "GST", 10, false)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+id, `{"name":"VAT","rate":20}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateEmptyName400(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, uuid, "GST", 10, false)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+id, `{"name":"","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	// Required-field validation moved into the service, so an empty name is now
	// a 422 (validation failed) rather than the old handler-level 400.
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: want 422 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateMissing404(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+uuidpkg.NewString(), `{"name":"X","rate":1}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestTaxRateDelete204(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, uuid, "GST", 10, false)
	resp := delete_(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestTaxRateExclusiveDefault(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	idA := createTaxRate(t, c, srv.URL, uuid, "A", 5, true)
	idB := createTaxRate(t, c, srv.URL, uuid, "B", 10, true)

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: want 200 got %d", resp.StatusCode)
	}
	var out []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	defaults := map[string]bool{}
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
	srv, uuid := newTaxRateServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}

func TestTaxRateCreateMalformedJSON400(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates", "{")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed JSON: want 400 got %d", resp.StatusCode)
	}
}

func TestTaxRateUpdateMalformedJSON400(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	id := createTaxRate(t, c, srv.URL, uuid, "GST", 10, false)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+id, "{")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed JSON update: want 400 got %d", resp.StatusCode)
	}
}

func TestTaxRateDeleteMissing404(t *testing.T) {
	srv, uuid := newTaxRateServer(t)
	c := loggedInClient(t, srv.URL)
	resp := delete_(t, c, srv.URL+"/api/t/"+uuid+"/tax-rates/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing: want 404 got %d", resp.StatusCode)
	}
}
