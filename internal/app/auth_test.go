package app

import (
	"database/sql"
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	// App integration tests seed whatever catalogue data they need, so start
	// from a clean catalogue.
	if _, err := conn.Exec("DELETE FROM catalogue_items"); err != nil {
		t.Fatalf("clear catalogue: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// seedTenantOwner provisions a tenant plus its platform-admin owner
// ("o@x.com" / "password1"). It returns the users repo, the tenant id, the owner
// user id, and the tenant's public UUID. End-to-end login + RequireSession +
// ResolveTenant wire the tenant into context from the {tenantUUID} URL segment,
// so callers build request URLs as /api/t/<uuid>/<resource>.
func seedTenantOwner(t *testing.T, conn *sql.DB) (*auth.UsersRepo, string, string, string) {
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
	// Tenant ids are now uuid strings, so the tenant id and its public uuid are
	// one and the same value.
	return users, tn.ID, owner.ID, tn.ID
}

// newAuthServer spins up a real httptest.Server wrapping the session middleware
// so cookies round-trip. It returns the server, the users repo, the owner id,
// the tenant id (for tenant-scoped deletes) and the tenant's public UUID (for
// building /api/t/<uuid>/... request URLs).
func newAuthServer(t *testing.T) (*httptest.Server, *auth.UsersRepo, string, string, string) {
	t.Helper()
	conn := openMigratedDB(t, "a.db")
	users, tenantID, ownerID, tenantUUID := seedTenantOwner(t, conn)

	sm := auth.NewSessionManager(conn, false)
	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(sm, users, tenants)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Post("/auth/logout", authH.Logout)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Get("/auth/me", authH.Me)
			pr.Get("/probe", probe200)
		})
	})

	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, users, ownerID, tenantID, tenantUUID
}

func probe200(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	srv, _, _, _, _ := newAuthServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "o@x.com", "wrong")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: want 401 got %d", resp.StatusCode)
	}
}

func TestAuthLoginCorrectReturnsUser(t *testing.T) {
	srv, _, _, _, _ := newAuthServer(t)
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
	srv, _, _, _, uuid := newAuthServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
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
	srv, _, _, _, uuid := newAuthServer(t)
	c := jarClient(t) // fresh, no login
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me no cookie: want 401 got %d", resp.StatusCode)
	}
}

func TestAuthProbeGuard(t *testing.T) {
	srv, _, _, _, uuid := newAuthServer(t)

	anon := jarClient(t)
	respAnon := get(t, anon, srv.URL+"/api/t/"+uuid+"/probe")
	_ = respAnon.Body.Close()
	if respAnon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("probe anon: want 401 got %d", respAnon.StatusCode)
	}

	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/probe")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("probe authed: want 200 got %d", resp.StatusCode)
	}
}

func TestAuthDeletedUserSessionRejected(t *testing.T) {
	srv, users, id, tenantID, uuid := newAuthServer(t)
	c := loggedInClient(t, srv.URL)

	// Confirm the session works first.
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-delete me: want 200 got %d", resp.StatusCode)
	}

	if err := users.Delete(t.Context(), tenantID, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Same cookie. The session (userID+email) still passes RequireSession, but
	// ResolveTenant's GetByEmail now finds no member for this tenant → 403.
	// (Under the old tenant-in-session model the user-exists recheck returned
	// 401; URL-tenant authorization surfaces a deleted member as 403.)
	resp2 := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("deleted-user me: want 403 got %d", resp2.StatusCode)
	}
}

// loginWithTenant performs a login carrying an explicit tenant uuid (the 409
// disambiguation re-submit) and returns the response.
func loginWithTenant(t *testing.T, c *http.Client, base, email, password, tenantUUID string) *http.Response {
	t.Helper()
	body := strings.NewReader(`{"email":"` + email + `","password":"` + password +
		`","tenantId":"` + tenantUUID + `"}`)
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

// TestAuthLoginMultiTenantByUUID exercises the multi-tenant disambiguation: an
// email registered in two tenants returns 409 with the candidate tenant uuids;
// re-submitting with one tenant's uuid logs in to that tenant. A bogus uuid 401s.
func TestAuthLoginMultiTenantByUUID(t *testing.T) {
	conn := openMigratedDB(t, "a.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Two tenants share the email "dup@x.com" → ambiguous global lookup.
	t1, err := tenants.Create(t.Context(), "Acme")
	if err != nil {
		t.Fatalf("Create t1: %v", err)
	}
	t2, err := tenants.Create(t.Context(), "Beta")
	if err != nil {
		t.Fatalf("Create t2: %v", err)
	}
	if _, err := users.Create(t.Context(), t1.ID, "dup@x.com", hash, "", "owner", false); err != nil {
		t.Fatalf("Create user t1: %v", err)
	}
	if _, err := users.Create(t.Context(), t2.ID, "dup@x.com", hash, "", "owner", false); err != nil {
		t.Fatalf("Create user t2: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, tenants)
	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
	})
	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)

	// Step 1: login without a tenant → 409 with the candidate tenant uuids.
	c := jarClient(t)
	resp := login(t, c, srv.URL, "dup@x.com", "password1")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("ambiguous login: want 409 got %d", resp.StatusCode)
	}
	var body struct {
		TenantRequired bool `json:"tenantRequired"`
		Tenants        []struct {
			ID string `json:"id"`
		} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode 409: %v", err)
	}
	if !body.TenantRequired || len(body.Tenants) != 2 {
		t.Fatalf("409 body unexpected: %+v", body)
	}
	for _, tn := range body.Tenants {
		if _, perr := uuid.Parse(tn.ID); perr != nil {
			t.Fatalf("409 tenant id not a uuid: %q", tn.ID)
		}
	}

	// Step 2: re-submit with t2's uuid → 200, logged into t2.
	resp2 := loginWithTenant(t, c, srv.URL, "dup@x.com", "password1", t2.ID)
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("re-submit by uuid: want 200 got %d", resp2.StatusCode)
	}
	u := decodeUser(t, resp2)
	if u.TenantID != t2.ID {
		t.Fatalf("logged into wrong tenant: want %s got %s", t2.ID, u.TenantID)
	}

	// Step 3: a well-formed but unknown tenant uuid → 401 (no enumeration).
	c2 := jarClient(t)
	resp3 := loginWithTenant(t, c2, srv.URL, "dup@x.com", "password1", ids.New())
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bogus tenant uuid: want 401 got %d", resp3.StatusCode)
	}
}

func TestAuthLogoutInvalidatesSession(t *testing.T) {
	srv, _, _, _, uuid := newAuthServer(t)
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

	resp2 := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-logout me: want 401 got %d", resp2.StatusCode)
	}
}

// TestLoginEmailCaseInsensitive guards the signup/login casing mismatch: signup
// stores the email lower-cased, so login must normalize too. Pre-fix, logging in
// with the same email in a different case 401'd ("invalid credentials").
func TestLoginEmailCaseInsensitive(t *testing.T) {
	srv, _, _, _, _ := newAuthServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "O@X.COM", "password1") // seeded owner is o@x.com
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mixed-case email login should succeed (stored lower-cased): code=%d", resp.StatusCode)
	}
}
