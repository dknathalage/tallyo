package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
)

func TestRecoverTurnsPanicInto500(t *testing.T) {
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	// LoggerFrom falls back to slog.Default() when no logger is in context, so no
	// seeding is required here.
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500 got %d", rec.Code)
	}
}

func TestRequestLoggerCapturesStatus(t *testing.T) {
	h := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("status not passed through: got %d", rec.Code)
	}
}

func TestRequireRoleForbidsWrongRole(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) })
	h := RequireRole("owner")(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, &auth.User{Role: "member"}))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestRequireRoleAllowsListedRole(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { called = true; w.WriteHeader(200) })
	h := RequireRole("owner", "admin")(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, &auth.User{Role: "admin"}))
	h.ServeHTTP(rec, req)
	if !called || rec.Code != 200 {
		t.Fatalf("admin should pass: called=%v code=%d", called, rec.Code)
	}
}

func TestRequireRoleNoUser401(t *testing.T) {
	h := RequireRole("owner")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", rec.Code)
	}
}

func TestRequirePlatformAdminForbidsNonAdmin(t *testing.T) {
	h := RequirePlatformAdmin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), userCtxKey, &auth.User{IsPlatformAdmin: false}))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403 got %d", rec.Code)
	}
}

func TestRequireSession401WithoutSession(t *testing.T) {
	// scs's GetString panics on a bare context ("no session data in context"),
	// so the handler must run inside LoadAndSave. With no cookie the loaded session
	// is empty → userID/email "" → 401.
	sm := scs.New()
	h := RequireSession(sm)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // should never run
	}))
	srv := httptest.NewServer(sm.LoadAndSave(h))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401 got %d", resp.StatusCode)
	}
}
