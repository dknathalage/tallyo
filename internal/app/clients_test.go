package app

import (
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/client"
	"github.com/dknathalage/tallyo/internal/payer"
	"github.com/go-chi/chi/v5"
	uuidpkg "github.com/google/uuid"
)

// newClientServer wires the client routes behind RequireSession + ResolveTenant plus the
// payer create route so a client can reference a payer FK.
func newClientServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "client.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)

	v := newStubVerifier()
	tenants := auth.NewTenants(conn)
	clientSvc := client.NewService(conn)
	pH := client.NewHandler(clientSvc)
	pmH := payer.NewHandler(payer.NewService(conn))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants, false))
			pH.Routes(pr)
			pr.Post("/payers", pmH.Create)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, tenantUUID
}

// createClient posts a client with the given name and returns its uuid.
func createClient(t *testing.T, c *http.Client, base, uuid, name string) string {
	t.Helper()
	resp := postJSON(t, c, base+"/api/t/"+uuid+"/clients", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create client %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode client: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("create client: want non-empty uuid got %q", out.ID)
	}
	return out.ID
}

func TestClientListEmptyReturnsArray(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/clients")
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

func TestClientListPayerName(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	pmID := createPayer(t, c, srv.URL, uuid, "Globex")

	body, err := json.Marshal(map[string]any{"name": "Wayne", "payerId": pmID})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients", string(body))
	if resp.StatusCode != http.StatusCreated {
		_ = resp.Body.Close()
		t.Fatalf("create client: want 201 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	lr := get(t, c, srv.URL+"/api/t/"+uuid+"/clients")
	defer func() { _ = lr.Body.Close() }()
	var out []struct {
		Name      string `json:"name"`
		PayerName string `json:"payerName"`
	}
	if err := json.NewDecoder(lr.Body).Decode(&out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 client got %d", len(out))
	}
	if out[0].PayerName != "Globex" {
		t.Fatalf("payerName: want Globex got %q", out[0].PayerName)
	}
}

func TestClientCreateWithFieldsAndGet(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	pmID := createPayer(t, c, srv.URL, uuid, "Acme")
	body, err := json.Marshal(map[string]any{
		"name": "Stark", "reference": "430000001",
		"payerId": pmID, "email": "s@x.com", "phone": "123", "address": "1 St",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients", string(body))
	if resp.StatusCode != http.StatusCreated {
		_ = resp.Body.Close()
		t.Fatalf("create: want 201 got %d", resp.StatusCode)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		_ = resp.Body.Close()
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()

	gr := get(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+out.ID)
	defer func() { _ = gr.Body.Close() }()
	if gr.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", gr.StatusCode)
	}
	var p struct {
		Reference string `json:"reference"`
	}
	if err := json.NewDecoder(gr.Body).Decode(&p); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if p.Reference != "430000001" {
		t.Fatalf("reference: want 430000001 got %q", p.Reference)
	}
}

func TestClientGetNotFound404(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+uuidpkg.NewString())
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing: want 404 got %d", resp.StatusCode)
	}
}

func TestClientCreateEmptyName400(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	// Required-field validation moved into the service, so an empty name is now
	// a 422 (validation failed) rather than the old handler-level 400.
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty name: want 422 got %d", resp.StatusCode)
	}
}

func TestClientUpdateOK(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	id := createClient(t, c, srv.URL, uuid, "Stark")
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+id, `{"name":"Stark Industries"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestClientUpdateMissing404(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+uuidpkg.NewString(), `{"name":"Nope"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestClientUpdateEmptyName400(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	id := createClient(t, c, srv.URL, uuid, "Stark")
	resp := putJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+id, `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	// Required-field validation moved into the service, so an empty name is now
	// a 422 (validation failed) rather than the old handler-level 400.
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("update empty name: want 422 got %d", resp.StatusCode)
	}
}

func TestClientDelete204(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	id := createClient(t, c, srv.URL, uuid, "Stark")
	resp := delete_(t, c, srv.URL+"/api/t/"+uuid+"/clients/"+id)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestClientBulkDelete204(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	a := createClient(t, c, srv.URL, uuid, "A")
	b := createClient(t, c, srv.URL, uuid, "B")
	resp := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/clients/bulk-delete", `{"ids":["`+a+`","`+b+`"]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestClientListSearchFilters(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createClient(t, c, srv.URL, uuid, "Acme")
	_ = createClient(t, c, srv.URL, uuid, "Globex")

	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/clients?search=acm")
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

func TestClientListUnauthenticated401(t *testing.T) {
	srv, uuid := newClientServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/clients")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
