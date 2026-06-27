package app

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/go-chi/chi/v5"
)

// signupToken is the bearer token for a brand-new person signing up: the stub
// maps it to a fresh uid/email that no tenant yet knows.
const signupToken = "signup-token"

// newSignupServer wires the Bearer-authed signup route plus an authenticated
// /auth/me so tests can assert the new owner can reach their tenant.
func newSignupServer(t *testing.T) (*httptest.Server, *sql.DB, *auth.UsersRepo, *auth.TenantsRepo) {
	t.Helper()
	conn := openMigratedDB(t, "signup.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	signupH := NewSignupHandler(tenants, users, func(ctx context.Context, tenantID string, in auth.SignupInput) error {
		return auth.ProvisionBusinessProfile(ctx, conn, tenantID, in)
	})
	authH := NewAuthHandler(users, tenants)

	v := newStubVerifier()
	v.add(signupToken, auth.Token{UID: "uid-ada", Email: "ada@example.com", Name: "Ada"})

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Group(func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Post("/signup", signupH.Signup)
		})
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Get("/auth/me", authH.Me)
		})
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, conn, users, tenants
}

// tenantForEmail looks up the tenant uuid that owns the given user email. The
// signup response serializes the owner's user uuid but not the tenant uuid, so
// the test resolves it from the DB. The tenant id IS its uuid, so both returns
// are equal.
func tenantForEmail(t *testing.T, conn *sql.DB, email string) (string, string) {
	t.Helper()
	var id string
	if err := conn.QueryRowContext(t.Context(),
		"SELECT t.id FROM tenants t JOIN users u ON u.tenant_id = t.id WHERE u.email = $1",
		email).Scan(&id); err != nil {
		t.Fatalf("tenant lookup for %q: %v", email, err)
	}
	return id, id
}

func TestSignupHappyPathProvisions(t *testing.T) {
	srv, conn, _, tenants := newSignupServer(t)
	// Bearer-authed: the email/uid come from the token, the body carries only the
	// business name + display name.
	c := bearerClient(signupToken)

	resp := postJSON(t, c, srv.URL+"/api/signup", `{"businessName":"Acme Care","name":"Ada"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("signup: want 201 got %d", resp.StatusCode)
	}
	u := decodeUser(t, resp)
	if u.Role != "owner" || u.Email != "ada@example.com" || u.Name != "Ada" || u.ID == "" {
		t.Fatalf("signup owner wrong: %+v", u)
	}
	tenantID, tenantUUID := tenantForEmail(t, conn, u.Email)

	// Tenant exists and is active.
	status, ok, err := tenants.Status(t.Context(), tenantID)
	if err != nil || !ok || status != auth.StatusActive {
		t.Fatalf("tenant status: status=%q ok=%v err=%v", status, ok, err)
	}

	// business_profile created with the form's business name.
	var name string
	row := conn.QueryRowContext(t.Context(),
		"SELECT name FROM business_profile WHERE tenant_id = $1", tenantID)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("business_profile scan: %v", err)
	}
	if name != "Acme Care" {
		t.Fatalf("business_profile wrong: name=%q", name)
	}

	// The new owner can reach their tenant with the same token.
	me := get(t, c, srv.URL+"/api/t/"+tenantUUID+"/auth/me")
	defer func() { _ = me.Body.Close() }()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("post-signup me: want 200 got %d", me.StatusCode)
	}
}

func TestSignupRequiresBusinessName(t *testing.T) {
	srv, _, _, _ := newSignupServer(t)
	c := bearerClient(signupToken)
	resp := postJSON(t, c, srv.URL+"/api/signup", `{"businessName":"","name":"Ada"}`)
	code := resp.StatusCode
	_ = resp.Body.Close()
	if code != http.StatusBadRequest {
		t.Fatalf("empty business name: want 400 got %d", code)
	}
}

func TestSignupUnauthenticated401(t *testing.T) {
	srv, _, _, _ := newSignupServer(t)
	c := jarClient(t) // no bearer token
	resp := postJSON(t, c, srv.URL+"/api/signup", `{"businessName":"Acme"}`)
	code := resp.StatusCode
	_ = resp.Body.Close()
	if code != http.StatusUnauthorized {
		t.Fatalf("anon signup: want 401 got %d", code)
	}
}

// TestSignupAtomicRollback asserts the unit of work is all-or-nothing: when
// Signup rejects before any insert (forced via an empty firebase uid), no orphan
// tenant is left behind.
func TestSignupAtomicRollback(t *testing.T) {
	conn := openMigratedDB(t, "signup_rollback.db")
	tenants := auth.NewTenants(conn)

	_, err := tenants.Signup(t.Context(), auth.SignupInput{
		BusinessName: "Rollback Co",
		Email:        "r@b.com",
		FirebaseUID:  "",
	}, nil)
	if err == nil {
		t.Fatal("Signup with empty firebase uid: want error, got nil")
	}
	var n int
	if err := conn.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM tenants").Scan(&n); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if n != 0 {
		t.Fatalf("rollback: tenants table should be empty, got %d", n)
	}
}

// newRoleServer wires a settings probe and a clients probe behind
// httpx.RequireRole("owner","admin") so role enforcement can be tested
// independently of the concrete handlers. Owner and member both authenticate via
// the shared stub tokens.
func newRoleServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "role.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	tn, _ := tenants.Create(t.Context(), "Roles")
	if _, err := users.Create(t.Context(), tn.ID, "o@x.com", "uid-owner", "O", "owner", false); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if _, err := users.Create(t.Context(), tn.ID, "member@x.com", "uid-member", "M", "member", false); err != nil {
		t.Fatalf("create member: %v", err)
	}

	v := newStubVerifier()
	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.With(httpx.RequireRole("owner", "admin")).Post("/settings", probe200)
			pr.Get("/clients", probe200) // any role
		})
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, tn.ID
}

func TestRoleMemberBlockedFromSettings(t *testing.T) {
	srv, uuid := newRoleServer(t)
	c := bearerClient(memberToken)
	// Member: blocked from settings (403), allowed on clients (200).
	settings := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/settings", `{}`)
	_ = settings.Body.Close()
	if settings.StatusCode != http.StatusForbidden {
		t.Fatalf("member settings: want 403 got %d", settings.StatusCode)
	}
	parts := get(t, c, srv.URL+"/api/t/"+uuid+"/clients")
	_ = parts.Body.Close()
	if parts.StatusCode != http.StatusOK {
		t.Fatalf("member clients: want 200 got %d", parts.StatusCode)
	}
}

func TestRoleOwnerAllowedOnSettings(t *testing.T) {
	srv, uuid := newRoleServer(t)
	c := bearerClient(ownerToken)
	settings := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/settings", `{}`)
	_ = settings.Body.Close()
	if settings.StatusCode != http.StatusOK {
		t.Fatalf("owner settings: want 200 got %d", settings.StatusCode)
	}
}
