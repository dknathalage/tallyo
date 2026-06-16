package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
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

// newPlanManagerServer wires the plan-manager routes behind RequireAuth the same
// way production does, plus a login route so tests can authenticate.
func newPlanManagerServer(t *testing.T) *httptest.Server {
	t.Helper()
	conn := openMigratedDB(t, "plan_manager.db")
	users, _, _ := seedTenantOwner(t, conn)

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)
	pH := NewPlanManagerHandler(service.NewPlanManagerService(conn, realtime.NewHub()))

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/plan-managers", pH.List)
			pr.Post("/plan-managers", pH.Create)
			pr.Post("/plan-managers/bulk-delete", pH.BulkDelete)
			pr.Get("/plan-managers/{id}", pH.Get)
			pr.Put("/plan-managers/{id}", pH.Update)
			pr.Delete("/plan-managers/{id}", pH.Delete)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv
}

// createPlanManager posts a plan manager with the given name and returns its id.
func createPlanManager(t *testing.T, c *http.Client, base, name string) int64 {
	t.Helper()
	resp := postJSON(t, c, base+"/api/plan-managers", `{"name":"`+name+`"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create plan manager %q: want 201 got %d", name, resp.StatusCode)
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode plan manager: %v", err)
	}
	if out.ID <= 0 {
		t.Fatalf("create plan manager: want id>0 got %d", out.ID)
	}
	return out.ID
}

func TestPlanManagerListEmptyReturnsArray(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/plan-managers")
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

func TestPlanManagerCreateAndGet(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPlanManager(t, c, srv.URL, "Acme")
	resp := get(t, c, srv.URL+"/api/plan-managers/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: want 200 got %d", resp.StatusCode)
	}
}

func TestPlanManagerGetNotFound404(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/plan-managers/99999")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing plan manager: want 404 got %d", resp.StatusCode)
	}
}

func TestPlanManagerGetBadID400(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/plan-managers/abc")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad id: want 400 got %d", resp.StatusCode)
	}
}

func TestPlanManagerCreateEmptyName400(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := postJSON(t, c, srv.URL+"/api/plan-managers", `{"name":""}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty name: want 400 got %d", resp.StatusCode)
	}
}

func TestPlanManagerUpdateOK(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPlanManager(t, c, srv.URL, "Acme")
	resp := putJSON(t, c, srv.URL+"/api/plan-managers/"+itoa(id), `{"name":"Acme Corp"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: want 200 got %d", resp.StatusCode)
	}
}

func TestPlanManagerUpdateMissing404(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	resp := putJSON(t, c, srv.URL+"/api/plan-managers/99999", `{"name":"Nope"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: want 404 got %d", resp.StatusCode)
	}
}

func TestPlanManagerDelete204(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	id := createPlanManager(t, c, srv.URL, "Acme")
	resp := delete_(t, c, srv.URL+"/api/plan-managers/"+itoa(id))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: want 204 got %d", resp.StatusCode)
	}
}

func TestPlanManagerBulkDelete204(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	a := createPlanManager(t, c, srv.URL, "A")
	b := createPlanManager(t, c, srv.URL, "B")
	resp := postJSON(t, c, srv.URL+"/api/plan-managers/bulk-delete", `{"ids":[`+itoa(a)+`,`+itoa(b)+`]}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bulk-delete: want 204 got %d", resp.StatusCode)
	}
}

func TestPlanManagerListSearchFilters(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := loggedInClient(t, srv.URL)
	_ = createPlanManager(t, c, srv.URL, "Acme")
	_ = createPlanManager(t, c, srv.URL, "Globex")

	resp := get(t, c, srv.URL+"/api/plan-managers?search=acm")
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

func TestPlanManagerListUnauthenticated401(t *testing.T) {
	srv := newPlanManagerServer(t)
	c := jarClient(t)
	resp := get(t, c, srv.URL+"/api/plan-managers")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401 got %d", resp.StatusCode)
	}
}
