package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

// fakeTenants implements TenantLookup from an in-memory map.
type fakeTenants struct{ byUUID map[string]*auth.Tenant }

func (f *fakeTenants) GetByUUID(_ context.Context, u string) (*auth.Tenant, error) {
	return f.byUUID[u], nil
}

// fakeUsers implements MemberLookup keyed by "tenantID\x00email".
type fakeUsers struct{ rows map[string]*auth.User }

func (f *fakeUsers) GetByEmail(_ context.Context, tenantID int64, email string) (*auth.User, error) {
	return f.rows[memberKey(tenantID, email)], nil
}

func memberKey(tenantID int64, email string) string {
	return string(rune(tenantID)) + "\x00" + email
}

// serve routes one GET through ResolveTenant (with email pre-attached as
// RequireSession would) and returns the recorded response + the resolved
// context captured by the terminal handler.
func serve(t *testing.T, users MemberLookup, tenants TenantLookup, tenantUUID, email string, final http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Route("/t/{tenantUUID}", func(tr chi.Router) {
		tr.Use(ResolveTenant(users, tenants))
		tr.Get("/x", final)
	})
	req := httptest.NewRequest(http.MethodGet, "/t/"+tenantUUID+"/x", nil)
	if email != "" {
		req = req.WithContext(reqctx.WithEmail(req.Context(), email))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestResolveTenant_MemberResolvesTenantAndRole(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{
		"uuid-a": {ID: 7, UUID: "uuid-a", Status: auth.StatusActive},
	}}
	users := &fakeUsers{rows: map[string]*auth.User{
		memberKey(7, "x@y.com"): {ID: 42, TenantID: 7, Email: "x@y.com", Role: "admin"},
	}}

	var gotTenant int64
	var gotUser *auth.User
	rec := serve(t, users, tenants, "uuid-a", "x@y.com", func(w http.ResponseWriter, r *http.Request) {
		gotTenant = reqctx.MustTenant(r.Context())
		gotUser = UserFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if gotTenant != 7 {
		t.Errorf("reqctx tenant = %d, want 7", gotTenant)
	}
	if gotUser == nil || gotUser.ID != 42 || gotUser.Role != "admin" {
		t.Errorf("resolved user = %+v, want id 42 role admin", gotUser)
	}
}

func TestResolveTenant_NonMemberForbidden(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{"uuid-a": {ID: 7, UUID: "uuid-a", Status: auth.StatusActive}}}
	users := &fakeUsers{rows: map[string]*auth.User{}} // email has no row in tenant 7

	ran := false
	rec := serve(t, users, tenants, "uuid-a", "x@y.com", func(w http.ResponseWriter, r *http.Request) { ran = true })

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
	if ran {
		t.Error("handler ran despite non-member")
	}
}

func TestResolveTenant_UnknownTenant404(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{}}
	users := &fakeUsers{rows: map[string]*auth.User{}}
	rec := serve(t, users, tenants, "nope", "x@y.com", func(w http.ResponseWriter, r *http.Request) {})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestResolveTenant_SuspendedForbidden(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{"uuid-a": {ID: 7, UUID: "uuid-a", Status: auth.StatusSuspended}}}
	users := &fakeUsers{rows: map[string]*auth.User{memberKey(7, "x@y.com"): {ID: 42, TenantID: 7, Role: "owner"}}}
	rec := serve(t, users, tenants, "uuid-a", "x@y.com", func(w http.ResponseWriter, r *http.Request) {})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestResolveTenant_MissingEmailUnauthorized(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{"uuid-a": {ID: 7, UUID: "uuid-a", Status: auth.StatusActive}}}
	users := &fakeUsers{rows: map[string]*auth.User{}}
	rec := serve(t, users, tenants, "uuid-a", "", func(w http.ResponseWriter, r *http.Request) {})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

// The same email has different roles in two tenants; RequireRole chained after
// ResolveTenant must reflect the URL tenant's role.
func TestResolveTenant_PerTenantRoleGate(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{
		"uuid-a": {ID: 1, UUID: "uuid-a", Status: auth.StatusActive},
		"uuid-b": {ID: 2, UUID: "uuid-b", Status: auth.StatusActive},
	}}
	users := &fakeUsers{rows: map[string]*auth.User{
		memberKey(1, "x@y.com"): {ID: 10, TenantID: 1, Email: "x@y.com", Role: "owner"},
		memberKey(2, "x@y.com"): {ID: 20, TenantID: 2, Email: "x@y.com", Role: "member"},
	}}

	run := func(tenantUUID string) int {
		r := chi.NewRouter()
		r.Route("/t/{tenantUUID}", func(tr chi.Router) {
			tr.Use(ResolveTenant(users, tenants))
			tr.With(RequireRole("owner")).Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		})
		req := httptest.NewRequest(http.MethodGet, "/t/"+tenantUUID+"/x", nil)
		req = req.WithContext(reqctx.WithEmail(req.Context(), "x@y.com"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := run("uuid-a"); code != http.StatusOK {
		t.Errorf("owner in tenant A: want 200, got %d", code)
	}
	if code := run("uuid-b"); code != http.StatusForbidden {
		t.Errorf("member in tenant B: want 403, got %d", code)
	}
}
