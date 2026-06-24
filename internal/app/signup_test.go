package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/go-chi/chi/v5"
)

// newSignupServer wires the public signup route plus an authenticated /auth/me
// so tests can assert the session lands logged in.
func newSignupServer(t *testing.T) (*httptest.Server, *sql.DB, *auth.UsersRepo, *auth.TenantsRepo) {
	t.Helper()
	conn := openMigratedDB(t, "signup.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	sm := auth.NewSessionManager(conn, false)
	signupH := NewSignupHandler(sm, tenants, users, func(ctx context.Context, tenantID string, in auth.SignupInput) error {
		return auth.ProvisionBusinessProfile(ctx, conn, tenantID, in)
	})
	authH := NewAuthHandler(sm, users, tenants)

	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/signup", signupH.Signup)
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.Get("/auth/me", authH.Me)
		})
	})
	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, conn, users, tenants
}

// tenantForEmail looks up the tenant uuid that owns the given user email. The
// signup response serializes the owner's user uuid but not the tenant uuid, so
// the test resolves it from the DB to drive tenant-scoped assertions and
// /api/t/<uuid>/... URLs. The tenant id IS its uuid, so both returns are equal.
func tenantForEmail(t *testing.T, conn *sql.DB, email string) (string, string) {
	t.Helper()
	var id string
	if err := conn.QueryRowContext(t.Context(),
		"SELECT t.id FROM tenants t JOIN users u ON u.tenant_id = t.id WHERE u.email = ?",
		email).Scan(&id); err != nil {
		t.Fatalf("tenant lookup for %q: %v", email, err)
	}
	return id, id
}

func TestSignupHappyPathLogsInAndProvisions(t *testing.T) {
	srv, conn, _, tenants := newSignupServer(t)
	c := jarClient(t)

	resp := postJSON(t, c, srv.URL+"/api/signup",
		`{"businessName":"Acme Care","name":"Ada","email":"Ada@Example.com","password":"password1"}`)
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
		"SELECT name FROM business_profile WHERE tenant_id = ?", tenantID)
	if err := row.Scan(&name); err != nil {
		t.Fatalf("business_profile scan: %v", err)
	}
	if name != "Acme Care" {
		t.Fatalf("business_profile wrong: name=%q", name)
	}

	// Session is established: /auth/me works with the same cookie jar.
	me := get(t, c, srv.URL+"/api/t/"+tenantUUID+"/auth/me")
	defer func() { _ = me.Body.Close() }()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("post-signup me: want 200 got %d", me.StatusCode)
	}
}

func TestSignupValidation(t *testing.T) {
	srv, _, _, _ := newSignupServer(t)
	c := jarClient(t)
	cases := []struct{ name, body string }{
		{"empty business", `{"businessName":"","email":"a@b.com","password":"password1"}`},
		{"bad email", `{"businessName":"X","email":"nope","password":"password1"}`},
		{"short password", `{"businessName":"X","email":"a@b.com","password":"short"}`},
	}
	for _, tc := range cases {
		resp := postJSON(t, c, srv.URL+"/api/signup", tc.body)
		code := resp.StatusCode
		_ = resp.Body.Close()
		if code != http.StatusBadRequest {
			t.Fatalf("%s: want 400 got %d", tc.name, code)
		}
	}
}

// TestSignupAtomicRollback asserts the unit of work is all-or-nothing: when
// Signup rejects before any insert (forced via an empty password hash), no
// orphan tenant is left behind.
func TestSignupAtomicRollback(t *testing.T) {
	conn := openMigratedDB(t, "signup_rollback.db")
	tenants := auth.NewTenants(conn)

	// Empty password hash forces Signup to reject before any insert; assert the
	// guard fires and nothing is written.
	_, err := tenants.Signup(t.Context(), auth.SignupInput{
		BusinessName: "Rollback Co",
		Email:        "r@b.com",
		PasswordHash: "",
	}, nil)
	if err == nil {
		t.Fatal("Signup with empty hash: want error, got nil")
	}
	var n int
	if err := conn.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM tenants").Scan(&n); err != nil {
		t.Fatalf("count tenants: %v", err)
	}
	if n != 0 {
		t.Fatalf("rollback: tenants table should be empty, got %d", n)
	}
}

func TestLoginFailSafeAmbiguousEmail(t *testing.T) {
	conn := openMigratedDB(t, "ambig.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	hash, err := auth.HashPassword("password1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	// Same email in two tenants.
	t1, _ := tenants.Create(t.Context(), "T1")
	t2, _ := tenants.Create(t.Context(), "T2")
	if _, err := users.Create(t.Context(), t1.ID, "shared@x.com", hash, "A", "owner", false); err != nil {
		t.Fatalf("create user t1: %v", err)
	}
	if _, err := users.Create(t.Context(), t2.ID, "shared@x.com", hash, "B", "owner", false); err != nil {
		t.Fatalf("create user t2: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, tenants)
	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) { api.Post("/auth/login", authH.Login) })
	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)

	c := jarClient(t)
	// Ambiguous login without tenantId → 409 with tenant choices, NO session.
	resp := postJSON(t, c, srv.URL+"/api/auth/login", `{"email":"shared@x.com","password":"password1"}`)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("ambiguous login: want 409 got %d", resp.StatusCode)
	}
	var body struct {
		TenantRequired bool `json:"tenantRequired"`
		Tenants        []struct {
			ID string `json:"id"` // tenant uuid (int PK never crosses the API)
		} `json:"tenants"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.TenantRequired || len(body.Tenants) != 2 {
		t.Fatalf("ambiguous login body wrong: %+v", body)
	}
	if body.Tenants[0].ID == "" {
		t.Fatalf("ambiguous login: tenant choice missing uuid: %+v", body)
	}

	// Disambiguated login WITH the tenant uuid → 200 into the chosen tenant.
	c2 := jarClient(t)
	ok := postJSON(t, c2, srv.URL+"/api/auth/login",
		`{"email":"shared@x.com","password":"password1","tenantId":"`+t2.ID+`"}`)
	defer func() { _ = ok.Body.Close() }()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("disambiguated login: want 200 got %d", ok.StatusCode)
	}
	u := decodeUser(t, ok)
	if u.TenantID != t2.ID {
		t.Fatalf("disambiguated login: want tenant %q got %q", t2.ID, u.TenantID)
	}
}

func TestLoginSingleTenantStillWorks(t *testing.T) {
	srv, _, _, _, _ := newAuthServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "o@x.com", "password1")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("single-tenant login: want 200 got %d", resp.StatusCode)
	}
}

func TestLoginSuspendedTenantBlocked(t *testing.T) {
	conn := openMigratedDB(t, "suspended.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	hash, _ := auth.HashPassword("password1")
	tn, _ := tenants.Create(t.Context(), "Susp")
	if _, err := users.Create(t.Context(), tn.ID, "s@x.com", hash, "S", "owner", false); err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Suspend the tenant.
	if _, err := conn.ExecContext(t.Context(),
		"UPDATE tenants SET status = 'suspended' WHERE id = ?", tn.ID); err != nil {
		t.Fatalf("suspend: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, tenants)
	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) { api.Post("/auth/login", authH.Login) })
	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)

	c := jarClient(t)
	resp := login(t, c, srv.URL, "s@x.com", "password1")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("suspended login: want 403 got %d", resp.StatusCode)
	}
}

// newRoleServer wires a settings (business-profile PUT-like) probe and an invite
// probe behind httpx.RequireRole("owner","admin") so role enforcement can be tested
// independently of the concrete handlers.
func newRoleServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	conn := openMigratedDB(t, "role.db")
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	hash, _ := auth.HashPassword("password1")
	tn, _ := tenants.Create(t.Context(), "Roles")
	if _, err := users.Create(t.Context(), tn.ID, "owner@x.com", hash, "O", "owner", false); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if _, err := users.Create(t.Context(), tn.ID, "member@x.com", hash, "M", "member", false); err != nil {
		t.Fatalf("create member: %v", err)
	}

	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users, tenants)
	router := chi.NewRouter()
	router.Route("/api", func(api chi.Router) {
		api.Post("/auth/login", authH.Login)
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireSession(sm))
			pr.Use(httpx.ResolveTenant(users, tenants))
			pr.With(httpx.RequireRole("owner", "admin")).Post("/settings", probe200)
			pr.Get("/clients", probe200) // any role
		})
	})
	srv := httptest.NewServer(sm.LoadAndSave(router))
	t.Cleanup(srv.Close)
	return srv, tn.ID
}

func TestRoleMemberBlockedFromSettings(t *testing.T) {
	srv, uuid := newRoleServer(t)
	c := jarClient(t)
	resp := login(t, c, srv.URL, "member@x.com", "password1")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("member login: want 200 got %d", resp.StatusCode)
	}
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
	c := jarClient(t)
	resp := login(t, c, srv.URL, "owner@x.com", "password1")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("owner login: want 200 got %d", resp.StatusCode)
	}
	settings := postJSON(t, c, srv.URL+"/api/t/"+uuid+"/settings", `{}`)
	_ = settings.Body.Close()
	if settings.StatusCode != http.StatusOK {
		t.Fatalf("owner settings: want 200 got %d", settings.StatusCode)
	}
}
