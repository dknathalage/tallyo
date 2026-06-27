package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// Stable test bearer tokens mapped by the stub verifier to known identities. The
// owner is the seeded tenant owner; the member is a non-owner used to exercise
// role-gated 403 paths (seeded by seedMember).
const (
	ownerToken  = "owner-token"
	memberToken = "member-token"
)

// stubVerifier is the test TokenVerifier: it maps bearer-token strings to their
// claims so the whole suite needs no real Firebase/GCP. Register a token with
// add() then send it as "Authorization: Bearer <token>".
type stubVerifier struct{ tokens map[string]auth.Token }

func newStubVerifier() *stubVerifier {
	return &stubVerifier{tokens: map[string]auth.Token{
		ownerToken:  {UID: "uid-owner", Email: "o@x.com", Name: "Owner"},
		memberToken: {UID: "uid-member", Email: "member@x.com", Name: "Member"},
	}}
}

func (s *stubVerifier) add(token string, tok auth.Token) { s.tokens[token] = tok }

func (s *stubVerifier) VerifyIDToken(_ context.Context, idToken string) (auth.Token, error) {
	if tok, ok := s.tokens[idToken]; ok {
		return tok, nil
	}
	return auth.Token{}, auth.ErrInvalidToken
}

// openMigratedDB opens the shared migrated Postgres test database. OpenTestDB
// truncates every table on open, so each test starts from a clean schema
// (including an empty catalogue). The name arg is retained for call-site
// compatibility and ignored.
func openMigratedDB(t *testing.T, _ string) *sql.DB {
	t.Helper()
	return appdb.OpenTestDB(t)
}

// seedTenantOwner provisions a tenant plus its platform-admin owner
// ("o@x.com" / firebase uid "uid-owner"). It returns the users repo, the tenant
// id, the owner user id, and the tenant's public UUID. Tenant-scoped routes wire
// the tenant into context from the {tenantUUID} URL segment, so callers build
// request URLs as /api/t/<uuid>/<resource>.
func seedTenantOwner(t *testing.T, conn *sql.DB) (*auth.UsersRepo, string, string, string) {
	t.Helper()
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	tn, err := tenants.Create(t.Context(), "Acme")
	if err != nil {
		t.Fatalf("Create tenant: %v", err)
	}
	owner, err := users.Create(t.Context(), tn.ID, "o@x.com", "uid-owner", "", "owner", true)
	if err != nil {
		t.Fatalf("Create owner: %v", err)
	}
	// Tenant ids are uuid strings, so the tenant id and its public uuid are one
	// and the same value.
	return users, tn.ID, owner.ID, tn.ID
}

// newAuthServer spins up a real httptest.Server wired with the bearer-auth
// middleware. It returns the server, the users repo, the owner id, the tenant id
// (for tenant-scoped deletes) and the tenant's public UUID (for building
// /api/t/<uuid>/... request URLs).
func newAuthServer(t *testing.T) (*httptest.Server, *auth.UsersRepo, string, string, string) {
	t.Helper()
	conn := openMigratedDB(t, "a.db")
	users, tenantID, ownerID, tenantUUID := seedTenantOwner(t, conn)

	tenants := auth.NewTenants(conn)
	authH := NewAuthHandler(users, tenants)
	v := newStubVerifier()

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Get("/auth/session", authH.Session)
		})
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants, false))
			pr.Get("/auth/me", authH.Me)
			pr.Get("/probe", probe200)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, users, ownerID, tenantID, tenantUUID
}

func probe200(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// bearerClient returns an http.Client whose every request carries the given
// bearer token (stateless auth — no cookie jar needed).
func bearerClient(token string) *http.Client {
	return &http.Client{Transport: bearerTransport{token: token, base: http.DefaultTransport}}
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.token != "" {
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return b.base.RoundTrip(r)
}

// loggedInClient returns a client authenticated as the seeded owner.
func loggedInClient(t *testing.T, _ string) *http.Client {
	t.Helper()
	return bearerClient(ownerToken)
}

// jarClient returns an UNAUTHENTICATED client (no bearer token). Named for
// call-site compatibility with the old cookie-jar helper; under stateless auth
// it simply sends no Authorization header, so protected routes answer 401.
func jarClient(_ *testing.T) *http.Client {
	return bearerClient("")
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

func TestAuthMeWithBearer(t *testing.T) {
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
	raw, _ := json.Marshal(u)
	if strings.Contains(strings.ToLower(string(raw)), "firebase") {
		t.Fatalf("me leaked firebase uid: %q", raw)
	}
}

func TestAuthMeWithoutBearer(t *testing.T) {
	srv, _, _, _, uuid := newAuthServer(t)
	c := bearerClient("") // no token
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me no token: want 401 got %d", resp.StatusCode)
	}
}

func TestAuthMeWithInvalidBearer(t *testing.T) {
	srv, _, _, _, uuid := newAuthServer(t)
	c := bearerClient("bogus-token")
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me bad token: want 401 got %d", resp.StatusCode)
	}
}

func TestAuthProbeGuard(t *testing.T) {
	srv, _, _, _, uuid := newAuthServer(t)

	anon := bearerClient("")
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

func TestAuthDeletedUserRejected(t *testing.T) {
	srv, users, id, tenantID, uuid := newAuthServer(t)
	c := loggedInClient(t, srv.URL)

	// Confirm access works first.
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-delete me: want 200 got %d", resp.StatusCode)
	}

	if err := users.Delete(t.Context(), tenantID, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// The token still verifies (stateless), but ResolveTenant's firebase-uid
	// lookup now finds no member for this tenant → 403.
	resp2 := get(t, c, srv.URL+"/api/t/"+uuid+"/auth/me")
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("deleted-user me: want 403 got %d", resp2.StatusCode)
	}
}

// TestAuthSessionListsTenants exercises GET /api/auth/session: it returns the
// token's email and the tenants its uid belongs to.
func TestAuthSessionListsTenants(t *testing.T) {
	srv, _, _, _, _ := newAuthServer(t)
	c := loggedInClient(t, srv.URL)
	resp := get(t, c, srv.URL+"/api/auth/session")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("session: want 200 got %d", resp.StatusCode)
	}
	var body struct {
		Email   string `json:"email"`
		Tenants []struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if body.Email != "o@x.com" {
		t.Fatalf("session email=%q want o@x.com", body.Email)
	}
	if len(body.Tenants) != 1 || body.Tenants[0].Role != "owner" {
		t.Fatalf("session tenants=%+v", body.Tenants)
	}
}
