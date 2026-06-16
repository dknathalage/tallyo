package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dknathalage/tallyo/internal/auth"
	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/go-chi/chi/v5"
)

func newSetupRouter(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	users := auth.NewUsers(conn)
	tenants := auth.NewTenants(conn)
	h, err := NewSetupHandler(users, tenants)
	if err != nil {
		t.Fatalf("NewSetupHandler: %v", err)
	}
	r := chi.NewRouter()
	r.Get("/api/setup/status", h.Status)
	r.Post("/api/setup", h.CreateOwner)
	return r, conn
}

func doStatus(t *testing.T, r http.Handler) (int, bool) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/setup/status", nil))
	var body struct {
		OwnerExists bool `json:"ownerExists"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("status decode: %v (body=%q)", err, w.Body.String())
	}
	return w.Code, body.OwnerExists
}

func doCreate(t *testing.T, r http.Handler, payload string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/setup", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestSetupStatusEmpty(t *testing.T) {
	r, _ := newSetupRouter(t)
	code, exists := doStatus(t, r)
	if code != http.StatusOK || exists {
		t.Fatalf("status empty: code=%d ownerExists=%v", code, exists)
	}
}

func TestSetupCreateOwnerAndStatusFlips(t *testing.T) {
	r, _ := newSetupRouter(t)

	w := doCreate(t, r, `{"email":"owner@example.com","password":"supersecret"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create owner: code=%d body=%q", w.Code, w.Body.String())
	}
	var u auth.User
	if err := json.Unmarshal(w.Body.Bytes(), &u); err != nil {
		t.Fatalf("create decode: %v (body=%q)", err, w.Body.String())
	}
	if u.Email != "owner@example.com" || u.Role != "owner" || u.ID == 0 {
		t.Fatalf("created user wrong: %+v", u)
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "hash") ||
		strings.Contains(w.Body.String(), "$2") {
		t.Fatalf("response leaked password hash: %q", w.Body.String())
	}

	code, exists := doStatus(t, r)
	if code != http.StatusOK || !exists {
		t.Fatalf("status after create: code=%d ownerExists=%v", code, exists)
	}
}

func TestSetupSecondCreateConflicts(t *testing.T) {
	r, _ := newSetupRouter(t)

	w := doCreate(t, r, `{"email":"owner@example.com","password":"supersecret"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: code=%d body=%q", w.Code, w.Body.String())
	}
	w2 := doCreate(t, r, `{"email":"another@example.com","password":"supersecret"}`)
	if w2.Code != http.StatusConflict {
		t.Fatalf("second create: want 409 got %d body=%q", w2.Code, w2.Body.String())
	}
}

func TestSetupValidation(t *testing.T) {
	r, _ := newSetupRouter(t)

	short := doCreate(t, r, `{"email":"owner@example.com","password":"short"}`)
	if short.Code != http.StatusBadRequest {
		t.Fatalf("short password: want 400 got %d", short.Code)
	}
	empty := doCreate(t, r, `{"email":"","password":"supersecret"}`)
	if empty.Code != http.StatusBadRequest {
		t.Fatalf("empty email: want 400 got %d", empty.Code)
	}
}
