package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
)

// stubVerifier is a TokenVerifier for tests: it maps known token strings to
// their claims and rejects everything else, so tests need no real GCP/Firebase.
type stubVerifier struct {
	tokens map[string]auth.Token
}

func (s stubVerifier) VerifyIDToken(_ context.Context, idToken string) (auth.Token, error) {
	if tok, ok := s.tokens[idToken]; ok {
		return tok, nil
	}
	return auth.Token{}, errors.New("invalid token")
}

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

func TestRequireAuth401WithoutBearer(t *testing.T) {
	v := stubVerifier{tokens: map[string]auth.Token{"good": {UID: "uid1", Email: "a@x.com"}}}
	h := RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // should never run
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no header: want 401 got %d", rec.Code)
	}
}

func TestRequireAuth401WithInvalidToken(t *testing.T) {
	v := stubVerifier{tokens: map[string]auth.Token{"good": {UID: "uid1", Email: "a@x.com"}}}
	h := RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer nope")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad token: want 401 got %d", rec.Code)
	}
}

func TestRequireAuthPassesValidToken(t *testing.T) {
	v := stubVerifier{tokens: map[string]auth.Token{"good": {UID: "uid1", Email: "a@x.com"}}}
	called := false
	h := RequireAuth(v)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer good")
	h.ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("valid token should pass: called=%v code=%d", called, rec.Code)
	}
}
