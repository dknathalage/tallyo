package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/go-chi/chi/v5"
)

// openMigratedDB opens a fresh migrated SQLite database in a temp dir.
func openMigratedDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// seedTenantOwner provisions a tenant plus its platform-admin owner
// ("o@x.com" / "password1"). It returns the users repo, the tenant id, and the
// owner user id. End-to-end login + RequireAuth wire the tenant into context, so
// callers only need the owner to exist in a tenant.
func seedTenantOwner(t *testing.T, conn *sql.DB) (*auth.UsersRepo, int64, int64) {
	t.Helper()
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	tn, err := tenants.Create(t.Context(), "Acme")
	if err != nil {
		t.Fatalf("Create tenant: %v", err)
	}
	owner, err := users.Create(t.Context(), tn.ID, "o@x.com", hash, "", "owner", true)
	if err != nil {
		t.Fatalf("Create owner: %v", err)
	}
	return users, tn.ID, owner.ID
}

// newAuthServer spins up a real httptest.Server wrapping the session middleware
// so cookies round-trip. It returns the server, the users repo, the owner id and
// the tenant id (the latter for tenant-scoped deletes).
func newAuthServer(t *testing.T) (*httptest.Server, *auth.UsersRepo, int64, int64) {
	t.Helper()
	conn := openMigratedDB(t, "a.db")
	users, tenantID, ownerID := seedTenantOwner(t, conn)

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Post("/auth/logout", authH.Logout)
		api.Group(func(pr chi.Router) {
			pr.Use(RequireAuth(sm, users))
			pr.Get("/auth/me", authH.Me)
			pr.Get("/probe", probe200)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, users, ownerID, tenantID
}

func probe200(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// jarClient returns an http.Client with a cookie jar so sessions persist.
func jarClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	return &http.Client{Jar: jar}
}

// login performs a login on the given client and returns the response.
func login(t *testing.T, c *http.Client, base, email, password string) *http.Response {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password + `"}`)
	req, err := http.NewRequest("POST", base+"/api/auth/login", body)
	if err != nil {
		t.Fatalf("new login req: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("login do: %v", err)
	}
	return resp
}

// loggedInClient returns a cookiejar client that has logged in as the owner.
func loggedInClient(t *testing.T, base string) *http.Client {
	t.Helper()
	c := jarClient(t)
	resp := login(t, c, base, "o@x.com", "password1")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("loggedInClient: login code=%d", resp.StatusCode)
	}
	return c
}

func decodeUser(t *testing.T, resp *http.Response) auth.User {
	t.Helper()
	var u auth.User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		t.Fatalf("decode user: %v", err)
	}
	return u
}

func get(t *testing.T, c *http.Client, url string) *http.Response {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("get %s: %v", url, err)
	}
	return resp
}

func TestAuthLoginWrongPassword(t *testing.T) {
	srv, _, _, _ := newAuthServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "o@x.com", "wrong")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: want 401 got %d", resp.StatusCode)
	}
}

func TestAuthLoginCorrectReturnsUser(t *testing.T) {
	srv, _, _, _ := newAuthServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "o@x.com", "password1")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: want 200 got %d", resp.StatusCode)
	}
	raw, _ := json.Marshal(decodeUser(t, resp))
	if strings.Contains(strings.ToLower(string(raw)), "hash") || strings.Contains(string(raw), "$2") {
		t.Fatalf("login leaked hash: %q", raw)
	}
	u := auth.User{}
	if err := json.Unmarshal(raw, &u); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if u.Email != "o@x.com" || u.Role != "owner" {
		t.Fatalf("login user wrong: %+v", u)
	}
}

func TestAuthMeWithCookie(t *testing.T) {
	srv, _, _, _ := newAuthServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/auth/me")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me: want 200 got %d", resp.StatusCode)
	}
	u := decodeUser(t, resp)
	if u.Email != "o@x.com" || u.Role != "owner" {
		t.Fatalf("me user wrong: %+v", u)
	}
}

func TestAuthMeWithoutCookie(t *testing.T) {
	srv, _, _, _ := newAuthServer(t)
	c := jarClient(t) // fresh, no login
	resp := get(t, c, srv.URL+"/api/auth/me")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me no cookie: want 401 got %d", resp.StatusCode)
	}
}

func TestAuthProbeGuard(t *testing.T) {
	srv, _, _, _ := newAuthServer(t)

	anon := jarClient(t)
	respAnon := get(t, anon, srv.URL+"/api/probe")
	_ = respAnon.Body.Close()
	if respAnon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("probe anon: want 401 got %d", respAnon.StatusCode)
	}

	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/probe")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe authed: want 200 got %d", resp.StatusCode)
	}
}

func TestAuthDeletedUserSessionRejected(t *testing.T) {
	srv, users, id, tenantID := newAuthServer(t)
	c := loggedInClient(t, srv.URL)

	// Confirm the session works first.
	resp := get(t, c, srv.URL+"/api/auth/me")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-delete me: want 200 got %d", resp.StatusCode)
	}

	if err := users.Delete(t.Context(), tenantID, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Same cookie, now the user no longer exists → 401.
	resp2 := get(t, c, srv.URL+"/api/auth/me")
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("deleted-user me: want 401 got %d", resp2.StatusCode)
	}
}

func TestAuthLogoutInvalidatesSession(t *testing.T) {
	srv, _, _, _ := newAuthServer(t)
	c := loggedInClient(t, srv.URL)

	req, err := http.NewRequest("POST", srv.URL+"/api/auth/logout", nil)
	if err != nil {
		t.Fatalf("logout req: %v", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("logout do: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: want 200 got %d", resp.StatusCode)
	}

	resp2 := get(t, c, srv.URL+"/api/auth/me")
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout me: want 401 got %d", resp2.StatusCode)
	}
}
