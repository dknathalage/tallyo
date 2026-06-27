package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/apperr"
	"github.com/dknathalage/tallyo/internal/audit"
	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/httpx"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/dknathalage/tallyo/internal/subscription"
	"github.com/go-chi/chi/v5"
)

// ── fake store implementations ───────────────────────────────────────────────

type fakeTenants struct {
	tenants    []*auth.TenantSummary
	byUUID     map[string]*auth.Tenant
	suspended  map[string]bool
	deleted    map[string]bool
	listErr    error
	getErr     error
	suspendErr error
	deleteErr  error
}

func newFakeTenants(t *auth.Tenant) *fakeTenants {
	f := &fakeTenants{
		byUUID:    make(map[string]*auth.Tenant),
		suspended: make(map[string]bool),
		deleted:   make(map[string]bool),
	}
	if t != nil {
		f.byUUID[t.ID] = t
		f.tenants = []*auth.TenantSummary{{Tenant: *t, UserCount: 1}}
	}
	return f
}

func (f *fakeTenants) List(_ context.Context) ([]*auth.TenantSummary, error) {
	return f.tenants, f.listErr
}

func (f *fakeTenants) GetByUUID(_ context.Context, uuid string) (*auth.Tenant, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.byUUID[uuid], nil
}

func (f *fakeTenants) Suspend(_ context.Context, uuid, _ string) error {
	if f.suspendErr != nil {
		return f.suspendErr
	}
	f.suspended[uuid] = true
	return nil
}

func (f *fakeTenants) Unsuspend(_ context.Context, uuid, _ string) error {
	f.suspended[uuid] = false
	return nil
}

func (f *fakeTenants) Delete(_ context.Context, uuid, _ string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted[uuid] = true
	return nil
}

type fakeSubscription struct {
	lastTenantID string
	lastStatus   string
	lastTrialEnd string
	err          error
}

func (f *fakeSubscription) SetSubscriptionStatus(_ context.Context, tenantID, status, _ string, trialEndsAt string) error {
	f.lastTenantID = tenantID
	f.lastStatus = status
	f.lastTrialEnd = trialEndsAt
	return f.err
}

// fakeAudit implements AuditLister, returning a fixed trail per tenant id.
type fakeAudit struct {
	byTenant map[string][]audit.Record
	err      error
}

func (f *fakeAudit) ListByTenant(_ context.Context, tenantID string) ([]audit.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byTenant[tenantID], nil
}

// newAdmin is a test constructor that defaults the audit lister to an empty fake
// so existing tests need not all pass one.
func newAdmin(tenants TenantsRepo, sub SubscriptionSetter) *Handler {
	return New(tenants, sub, &fakeAudit{byTenant: map[string][]audit.Record{}})
}

// ── helper: admin user in context ────────────────────────────────────────────

// withAdmin places an admin user on the request context the same way
// ResolveAdminUser + RequirePlatformAdmin would, using the exported helper.
func withAdmin(r *http.Request) *http.Request {
	admin := &auth.User{ID: "admin-user-id", IsPlatformAdmin: true, Role: "owner"}
	return r.WithContext(httpx.WithUserInContext(r.Context(), admin))
}

// withNonAdmin places a non-admin user on the request context (for 403 tests).
func withNonAdmin(r *http.Request) *http.Request {
	u := &auth.User{ID: "plain-user-id", IsPlatformAdmin: false, Role: "member"}
	return r.WithContext(httpx.WithUserInContext(r.Context(), u))
}

// ── List tests ───────────────────────────────────────────────────────────────

func TestListReturnsTenantsForAdmin(t *testing.T) {
	tenant := &auth.Tenant{ID: "t-1", Name: "Acme"}
	h := newAdmin(newFakeTenants(tenant), &fakeSubscription{})

	req := withAdmin(httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var body []*auth.TenantSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 1 || body[0].Name != "Acme" {
		t.Errorf("body = %+v, want one tenant named Acme", body)
	}
}

func TestListReturns500OnStoreError(t *testing.T) {
	ft := &fakeTenants{listErr: errors.New("db error")}
	h := newAdmin(ft, &fakeSubscription{})

	req := withAdmin(httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil))
	rec := httptest.NewRecorder()
	h.List(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
}

// ── Detail tests ─────────────────────────────────────────────────────────────

func chiReqWithUUID(method, path, uuidVal string, body []byte) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	// Inject chi URL params manually.
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("uuid", uuidVal)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestDetailReturnsTenantAndAuditForAdmin(t *testing.T) {
	tenant := &auth.Tenant{ID: "t-uuid-1", Name: "Beta"}
	ft := newFakeTenants(tenant)
	ft.byUUID["t-uuid-1"] = tenant
	fa := &fakeAudit{byTenant: map[string][]audit.Record{
		"t-uuid-1": {{ID: "a1", Action: "suspend", TenantID: "t-uuid-1", CreatedAt: "2026-06-27T10:00:00Z"}},
	}}
	h := New(ft, &fakeSubscription{}, fa)

	req := withAdmin(chiReqWithUUID(http.MethodGet, "/api/admin/tenants/t-uuid-1", "t-uuid-1", nil))
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Tenant *auth.Tenant   `json:"tenant"`
		Audit  []audit.Record `json:"audit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Tenant == nil || body.Tenant.Name != "Beta" {
		t.Errorf("body.Tenant = %+v, want name Beta", body.Tenant)
	}
	if len(body.Audit) != 1 || body.Audit[0].Action != "suspend" {
		t.Errorf("body.Audit = %+v, want one suspend row", body.Audit)
	}
}

func TestDetail404ForUnknownTenant(t *testing.T) {
	h := newAdmin(newFakeTenants(nil), &fakeSubscription{})
	req := withAdmin(chiReqWithUUID(http.MethodGet, "/api/admin/tenants/no-such", "no-such", nil))
	rec := httptest.NewRecorder()
	h.Detail(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// ── SetSubscription tests ─────────────────────────────────────────────────────

func TestSetSubscriptionPatchesStatus(t *testing.T) {
	fs := &fakeSubscription{}
	h := newAdmin(newFakeTenants(&auth.Tenant{ID: "t-2"}), fs)

	body, _ := json.Marshal(setSubscriptionRequest{Status: "active"})
	req := withAdmin(chiReqWithUUID(http.MethodPatch, "/api/admin/tenants/t-2/subscription", "t-2", body))
	rec := httptest.NewRecorder()
	h.SetSubscription(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if fs.lastStatus != "active" || fs.lastTenantID != "t-2" {
		t.Errorf("store called with status=%q tenantID=%q, want active/t-2", fs.lastStatus, fs.lastTenantID)
	}
}

func TestSetSubscriptionTrialingForwardsTrialEnd(t *testing.T) {
	fs := &fakeSubscription{}
	h := newAdmin(newFakeTenants(&auth.Tenant{ID: "t-3"}), fs)

	body, _ := json.Marshal(setSubscriptionRequest{Status: "trialing", TrialEndsAt: "2027-01-01T00:00:00Z"})
	req := withAdmin(chiReqWithUUID(http.MethodPatch, "/api/admin/tenants/t-3/subscription", "t-3", body))
	rec := httptest.NewRecorder()
	h.SetSubscription(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if fs.lastTrialEnd != "2027-01-01T00:00:00Z" {
		t.Errorf("trialEndsAt = %q, want 2027-01-01T00:00:00Z", fs.lastTrialEnd)
	}
}

func TestSetSubscriptionRejectsMissingStatus(t *testing.T) {
	h := newAdmin(newFakeTenants(nil), &fakeSubscription{})
	body, _ := json.Marshal(map[string]string{"status": ""})
	req := withAdmin(chiReqWithUUID(http.MethodPatch, "/api/admin/tenants/t-x/subscription", "t-x", body))
	rec := httptest.NewRecorder()
	h.SetSubscription(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rec.Code)
	}
}

// TestSetSubscriptionMalformedBody asserts a non-JSON body is rejected with 400
// before any store call.
func TestSetSubscriptionMalformedBody(t *testing.T) {
	fs := &fakeSubscription{}
	h := newAdmin(newFakeTenants(&auth.Tenant{ID: "t-x"}), fs)
	req := withAdmin(chiReqWithUUID(http.MethodPatch, "/api/admin/tenants/t-x/subscription", "t-x", []byte("not json{")))
	rec := httptest.NewRecorder()
	h.SetSubscription(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if fs.lastStatus != "" {
		t.Errorf("store should not be called on malformed body, got status %q", fs.lastStatus)
	}
}

// TestSetSubscriptionInvalidStatusMapsTo422 asserts that a store-layer
// apperr.Validation error (e.g. an invalid status) surfaces as 422, not 500.
func TestSetSubscriptionInvalidStatusMapsTo422(t *testing.T) {
	fs := &fakeSubscription{err: &apperr.ValidationError{Errors: []apperr.FieldError{{Field: "status", Message: "invalid status \"bogus\""}}}}
	h := newAdmin(newFakeTenants(&auth.Tenant{ID: "t-x"}), fs)
	body, _ := json.Marshal(setSubscriptionRequest{Status: "bogus"})
	req := withAdmin(chiReqWithUUID(http.MethodPatch, "/api/admin/tenants/t-x/subscription", "t-x", body))
	rec := httptest.NewRecorder()
	h.SetSubscription(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestSuspendUnknownTenantMapsTo404 asserts an apperr.ErrNotFound from the store
// surfaces as 404, not 500 or a silent 204.
func TestSuspendUnknownTenantMapsTo404(t *testing.T) {
	ft := newFakeTenants(nil)
	ft.suspendErr = fmt.Errorf("suspend tenant: %w", apperr.ErrNotFound)
	h := newAdmin(ft, &fakeSubscription{})
	req := withAdmin(chiReqWithUUID(http.MethodPost, "/api/admin/tenants/ghost/suspend", "ghost", nil))
	rec := httptest.NewRecorder()
	h.Suspend(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// TestDeleteUnknownTenantMapsTo404 mirrors the suspend case for Delete.
func TestDeleteUnknownTenantMapsTo404(t *testing.T) {
	ft := newFakeTenants(nil)
	ft.deleteErr = fmt.Errorf("delete tenant: %w", apperr.ErrNotFound)
	h := newAdmin(ft, &fakeSubscription{})
	req := withAdmin(chiReqWithUUID(http.MethodDelete, "/api/admin/tenants/ghost", "ghost", nil))
	rec := httptest.NewRecorder()
	h.Delete(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

// ── Suspend / Unsuspend tests ────────────────────────────────────────────────

func TestSuspendCallsThrough(t *testing.T) {
	ft := newFakeTenants(&auth.Tenant{ID: "t-4"})
	h := newAdmin(ft, &fakeSubscription{})

	req := withAdmin(chiReqWithUUID(http.MethodPost, "/api/admin/tenants/t-4/suspend", "t-4", nil))
	rec := httptest.NewRecorder()
	h.Suspend(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if !ft.suspended["t-4"] {
		t.Error("Suspend not called on store")
	}
}

func TestUnsuspendCallsThrough(t *testing.T) {
	ft := newFakeTenants(&auth.Tenant{ID: "t-5"})
	ft.suspended["t-5"] = true
	h := newAdmin(ft, &fakeSubscription{})

	req := withAdmin(chiReqWithUUID(http.MethodPost, "/api/admin/tenants/t-5/unsuspend", "t-5", nil))
	rec := httptest.NewRecorder()
	h.Unsuspend(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if ft.suspended["t-5"] {
		t.Error("Unsuspend did not clear suspended flag")
	}
}

// ── Delete tests ─────────────────────────────────────────────────────────────

func TestDeleteCallsThrough(t *testing.T) {
	ft := newFakeTenants(&auth.Tenant{ID: "t-6"})
	h := newAdmin(ft, &fakeSubscription{})

	req := withAdmin(chiReqWithUUID(http.MethodDelete, "/api/admin/tenants/t-6", "t-6", nil))
	rec := httptest.NewRecorder()
	h.Delete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d", rec.Code)
	}
	if !ft.deleted["t-6"] {
		t.Error("Delete not called on store")
	}
}

// ── RequirePlatformAdmin gate (403 for non-admin) ────────────────────────────

// TestNonAdminForbiddenViaMiddleware verifies the full middleware chain rejects
// a non-platform-admin with 403.
func TestNonAdminForbiddenViaMiddleware(t *testing.T) {
	h := newAdmin(newFakeTenants(nil), &fakeSubscription{})
	// Wrap the List handler with RequirePlatformAdmin (as it is in production).
	handler := httpx.RequirePlatformAdmin(http.HandlerFunc(h.List))

	req := withNonAdmin(httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rec.Code)
	}
}

// TestAdminPassesMiddleware verifies the full middleware chain admits a
// platform-admin.
func TestAdminPassesMiddleware(t *testing.T) {
	ft := newFakeTenants(&auth.Tenant{ID: "t-ok", Name: "OK Co"})
	h := newAdmin(ft, &fakeSubscription{})
	handler := httpx.RequirePlatformAdmin(http.HandlerFunc(h.List))

	req := withAdmin(httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

// ── DB-backed integration test ────────────────────────────────────────────────

// TestDBListAndSuspendAndDelete runs the full handler stack against a real
// Postgres DB (skipped when TEST_DATABASE_URL is unset).
func TestDBListAndSuspendAndDelete(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	// Create a platform admin user and a target tenant.
	adminTenant, err := auth.NewTenants(conn).Create(ctx, "Admin Home")
	if err != nil {
		t.Fatalf("create admin tenant: %v", err)
	}
	adminUser, err := auth.NewUsers(conn).Create(ctx, adminTenant.ID,
		"admin@tallyo.test", "uid-admin", "Admin", "owner", true)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetTenant, err := auth.NewTenants(conn).Create(ctx, "Target Co")
	if err != nil {
		t.Fatalf("create target tenant: %v", err)
	}

	subStore := subscription.NewStore(conn)
	h := New(auth.NewTenants(conn), subStore, audit.NewReader(conn))

	// List: target tenant must appear.
	{
		req := withAdminUser(adminUser, httptest.NewRequest(http.MethodGet, "/api/admin/tenants", nil))
		rec := httptest.NewRecorder()
		h.List(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("List: want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var body []*auth.TenantSummary
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("List decode: %v", err)
		}
		found := false
		for _, s := range body {
			if s.Name == "Target Co" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List did not return Target Co; got %d rows", len(body))
		}
	}

	// Detail: resolve by UUID and decode the {tenant, audit} response shape.
	{
		req := withAdminUser(adminUser, chiReqWithUUID(http.MethodGet, "/api/admin/tenants/"+targetTenant.ID, targetTenant.ID, nil))
		rec := httptest.NewRecorder()
		h.Detail(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Detail: want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var detail struct {
			Tenant *auth.Tenant   `json:"tenant"`
			Audit  []audit.Record `json:"audit"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
			t.Fatalf("Detail decode: %v", err)
		}
		if detail.Tenant == nil || detail.Tenant.ID != targetTenant.ID {
			t.Errorf("Detail tenant = %+v, want id %s", detail.Tenant, targetTenant.ID)
		}
	}

	// SetSubscription: patch to active.
	{
		body, _ := json.Marshal(setSubscriptionRequest{Status: "active"})
		req := withAdminUser(adminUser, chiReqWithUUID(http.MethodPatch,
			"/api/admin/tenants/"+targetTenant.ID+"/subscription", targetTenant.ID, body))
		rec := httptest.NewRecorder()
		h.SetSubscription(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("SetSubscription: want 204, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	// Suspend: status must become suspended.
	{
		req := withAdminUser(adminUser, chiReqWithUUID(http.MethodPost,
			"/api/admin/tenants/"+targetTenant.ID+"/suspend", targetTenant.ID, nil))
		rec := httptest.NewRecorder()
		h.Suspend(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("Suspend: want 204, got %d: %s", rec.Code, rec.Body.String())
		}
		status, found, err := auth.NewTenants(conn).Status(ctx, targetTenant.ID)
		if err != nil || !found {
			t.Fatalf("Status after Suspend: err=%v found=%v", err, found)
		}
		if status != auth.StatusSuspended {
			t.Errorf("status after Suspend = %q, want suspended", status)
		}
	}

	// Unsuspend: status must return to active.
	{
		req := withAdminUser(adminUser, chiReqWithUUID(http.MethodPost,
			"/api/admin/tenants/"+targetTenant.ID+"/unsuspend", targetTenant.ID, nil))
		rec := httptest.NewRecorder()
		h.Unsuspend(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("Unsuspend: want 204, got %d: %s", rec.Code, rec.Body.String())
		}
		status, found, err := auth.NewTenants(conn).Status(ctx, targetTenant.ID)
		if err != nil || !found {
			t.Fatalf("Status after Unsuspend: err=%v found=%v", err, found)
		}
		if status != auth.StatusActive {
			t.Errorf("status after Unsuspend = %q, want active", status)
		}
	}

	// Detail audit trail: by now the target tenant has set_subscription_status +
	// suspend + unsuspend audit rows. Detail must surface them (newest first),
	// attributed to the acting admin.
	{
		req := withAdminUser(adminUser, chiReqWithUUID(http.MethodGet, "/api/admin/tenants/"+targetTenant.ID, targetTenant.ID, nil))
		rec := httptest.NewRecorder()
		h.Detail(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("Detail (audit): want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var detail struct {
			Tenant *auth.Tenant   `json:"tenant"`
			Audit  []audit.Record `json:"audit"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
			t.Fatalf("Detail (audit) decode: %v", err)
		}
		if len(detail.Audit) < 3 {
			t.Fatalf("audit trail = %d rows, want >= 3 (set_subscription_status, suspend, unsuspend)", len(detail.Audit))
		}
		// Newest first: created_at must be non-increasing.
		for i := 1; i < len(detail.Audit); i++ {
			if detail.Audit[i-1].CreatedAt < detail.Audit[i].CreatedAt {
				t.Errorf("audit not ordered newest-first at %d: %q < %q", i, detail.Audit[i-1].CreatedAt, detail.Audit[i].CreatedAt)
			}
		}
		// Every row is stamped to the target tenant and the acting admin.
		actions := map[string]bool{}
		for _, rec := range detail.Audit {
			actions[rec.Action] = true
			if rec.TenantID != targetTenant.ID {
				t.Errorf("audit row %s tenantId = %q, want %q", rec.Action, rec.TenantID, targetTenant.ID)
			}
			if rec.UserID != adminUser.ID {
				t.Errorf("audit row %s userId = %q, want admin %q", rec.Action, rec.UserID, adminUser.ID)
			}
		}
		for _, want := range []string{"set_subscription_status", "suspend", "unsuspend"} {
			if !actions[want] {
				t.Errorf("audit trail missing action %q", want)
			}
		}
	}

	// Delete: tenant must be gone.
	{
		req := withAdminUser(adminUser, chiReqWithUUID(http.MethodDelete,
			"/api/admin/tenants/"+targetTenant.ID, targetTenant.ID, nil))
		rec := httptest.NewRecorder()
		h.Delete(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("Delete: want 204, got %d: %s", rec.Code, rec.Body.String())
		}
		_, found, err := auth.NewTenants(conn).Status(ctx, targetTenant.ID)
		if err != nil {
			t.Fatalf("Status after Delete: %v", err)
		}
		if found {
			t.Error("tenant still exists after Delete")
		}
	}
}

// withAdminUser places the given user on the request context (test variant that
// takes a real *auth.User).
func withAdminUser(u *auth.User, r *http.Request) *http.Request {
	return r.WithContext(httpx.WithUserInContext(r.Context(), u))
}

// TestResolveAdminUserMiddleware verifies that ResolveAdminUser resolves the
// calling user from the Firebase-verified email placed on context by RequireAuth.
func TestResolveAdminUserMiddleware(t *testing.T) {
	conn := appdb.OpenTestDB(t)
	ctx := context.Background()

	adminTenant, err := auth.NewTenants(conn).Create(ctx, "Admin Tenant")
	if err != nil {
		t.Fatalf("create admin tenant: %v", err)
	}
	adminUser, err := auth.NewUsers(conn).Create(ctx, adminTenant.ID,
		"superadmin@tallyo.test", "uid-super", "Super", "owner", true)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	usersRepo := auth.NewUsers(conn)
	middleware := httpx.ResolveAdminUser(usersRepo)

	var capturedUser *auth.User
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUser = httpx.UserFrom(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Happy path: email matches an existing user.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(reqctx.WithEmail(req.Context(), adminUser.Email))
	rec := httptest.NewRecorder()
	middleware(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if capturedUser == nil || capturedUser.ID != adminUser.ID {
		t.Errorf("resolved user = %+v, want id %s", capturedUser, adminUser.ID)
	}

	// Email absent → 401.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	middleware(inner).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("no email: want 401, got %d", rec2.Code)
	}

	// Unknown email → 403.
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3 = req3.WithContext(reqctx.WithEmail(req3.Context(), "nobody@example.com"))
	rec3 := httptest.NewRecorder()
	middleware(inner).ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusForbidden {
		t.Fatalf("unknown email: want 403, got %d", rec3.Code)
	}
}
