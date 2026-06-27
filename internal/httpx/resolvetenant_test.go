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

// fakeUsers implements MemberLookup keyed by "tenantID\x00firebaseUID".
type fakeUsers struct{ rows map[string]*auth.User }

func (f *fakeUsers) GetByFirebaseUID(_ context.Context, tenantID string, uid string) (*auth.User, error) {
	return f.rows[memberKey(tenantID, uid)], nil
}

func memberKey(tenantID string, uid string) string {
	return tenantID + "\x00" + uid
}

// serve routes one GET through ResolveTenant (with the Firebase uid pre-attached
// as RequireAuth would) and returns the recorded response + the resolved context
// captured by the terminal handler. The uid arg doubles as the member key.
func serve(t *testing.T, users MemberLookup, tenants TenantLookup, tenantUUID, uid string, final http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	r.Route("/t/{tenantUUID}", func(tr chi.Router) {
		tr.Use(ResolveTenant(users, tenants, false))
		tr.Get("/x", final)
	})
	req := httptest.NewRequest(http.MethodGet, "/t/"+tenantUUID+"/x", nil)
	if uid != "" {
		req = req.WithContext(reqctx.WithFirebaseUID(req.Context(), uid))
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestResolveTenant_MemberResolvesTenantAndRole(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{
		"uuid-a": {ID: "t-7", Status: auth.StatusActive},
	}}
	users := &fakeUsers{rows: map[string]*auth.User{
		memberKey("t-7", "x@y.com"): {ID: "u-42", TenantID: "t-7", Email: "x@y.com", Role: "admin"},
	}}

	var gotTenant string
	var gotUser *auth.User
	rec := serve(t, users, tenants, "uuid-a", "x@y.com", func(w http.ResponseWriter, r *http.Request) {
		gotTenant = reqctx.MustTenant(r.Context())
		gotUser = UserFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if gotTenant != "t-7" {
		t.Errorf("reqctx tenant = %q, want t-7", gotTenant)
	}
	if gotUser == nil || gotUser.ID != "u-42" || gotUser.Role != "admin" {
		t.Errorf("resolved user = %+v, want id u-42 role admin", gotUser)
	}
}

func TestResolveTenant_NonMemberForbidden(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{"uuid-a": {ID: "t-7", Status: auth.StatusActive}}}
	users := &fakeUsers{rows: map[string]*auth.User{}} // email has no row in tenant t-7

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
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{"uuid-a": {ID: "t-7", Status: auth.StatusSuspended}}}
	users := &fakeUsers{rows: map[string]*auth.User{memberKey("t-7", "x@y.com"): {ID: "u-42", TenantID: "t-7", Role: "owner"}}}
	rec := serve(t, users, tenants, "uuid-a", "x@y.com", func(w http.ResponseWriter, r *http.Request) {})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

func TestResolveTenant_MissingEmailUnauthorized(t *testing.T) {
	tenants := &fakeTenants{byUUID: map[string]*auth.Tenant{"uuid-a": {ID: "t-7", Status: auth.StatusActive}}}
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
		"uuid-a": {ID: "t-1", Status: auth.StatusActive},
		"uuid-b": {ID: "t-2", Status: auth.StatusActive},
	}}
	users := &fakeUsers{rows: map[string]*auth.User{
		memberKey("t-1", "x@y.com"): {ID: "u-10", TenantID: "t-1", Email: "x@y.com", Role: "owner"},
		memberKey("t-2", "x@y.com"): {ID: "u-20", TenantID: "t-2", Email: "x@y.com", Role: "member"},
	}}

	run := func(tenantUUID string) int {
		r := chi.NewRouter()
		r.Route("/t/{tenantUUID}", func(tr chi.Router) {
			tr.Use(ResolveTenant(users, tenants, false))
			tr.With(RequireRole("owner")).Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		})
		req := httptest.NewRequest(http.MethodGet, "/t/"+tenantUUID+"/x", nil)
		req = req.WithContext(reqctx.WithFirebaseUID(req.Context(), "x@y.com"))
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
