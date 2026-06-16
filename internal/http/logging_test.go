package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/go-chi/chi/v5"
)

// captureLogs installs a JSON slog handler writing to an in-memory buffer as the
// default logger for the duration of the test, restoring the prior default on
// cleanup. The returned function parses the buffered output into records. A mutex
// guards the buffer because requests are served on separate goroutines.
func captureLogs(t *testing.T) func() []map[string]any {
	t.Helper()
	var mu sync.Mutex
	buf := &bytes.Buffer{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&lockedWriter{mu: &mu, w: buf}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		var recs []map[string]any
		for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
			if line == "" {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(line), &m); err != nil {
				t.Fatalf("parse log line %q: %v", line, err)
			}
			recs = append(recs, m)
		}
		return recs
	}
}

// lockedWriter serializes concurrent writes to the underlying buffer.
type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// newLoggingServer builds a server with the full Recover→RequestLogger→Session
// chain plus an authenticated probe, so the request logger and RequireAuth
// enrichment are exercised end to end.
func newLoggingServer(t *testing.T) (*httptest.Server, *scs.SessionManager, *auth.UsersRepo) {
	t.Helper()
	conn := openMigratedDB(t, "log.db")
	users, _, _ := seedTenantOwner(t, conn)
	sm := auth.NewSessionManager(conn, false)
	authH := NewAuthHandler(sm, users)

	router := chi.NewRouter()
	router.Use(Recover)
	router.Use(RequestLogger)
	router.Group(func(g chi.Router) {
		g.Use(sm.LoadAndSave)
		g.Route("/api", func(api chi.Router) {
			api.Post("/auth/login", authH.Login)
			api.Group(func(pr chi.Router) {
				pr.Use(RequireAuth(sm, users))
				pr.Get("/probe", probe200)
			})
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, sm, users
}

// findRequestRecord returns the "request" summary record for the given path.
func findRequestRecord(t *testing.T, recs []map[string]any, path string) map[string]any {
	t.Helper()
	for _, r := range recs {
		if r["msg"] == "request" && r["path"] == path {
			return r
		}
	}
	t.Fatalf("no request record for path %q in %v", path, recs)
	return nil
}

func TestRequestLoggerAttachesRequestIDAndTenantUser(t *testing.T) {
	read := captureLogs(t)
	srv, _, _ := newLoggingServer(t)

	c := jarClient(t)
	resp := login(t, c, srv.URL, "o@x.com", "password1")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: want 200 got %d", resp.StatusCode)
	}
	pr := get(t, c, srv.URL+"/api/probe")
	_ = pr.Body.Close()
	if pr.StatusCode != http.StatusOK {
		t.Fatalf("probe: want 200 got %d", pr.StatusCode)
	}

	rec := findRequestRecord(t, read(), "/api/probe")

	for _, field := range []string{"request_id", "method", "path", "status", "duration_ms"} {
		if _, ok := rec[field]; !ok {
			t.Fatalf("request record missing %q: %v", field, rec)
		}
	}
	if rid, _ := rec["request_id"].(string); rid == "" {
		t.Fatalf("request_id empty: %v", rec)
	}
	// Authenticated request → tenant_id and user_id present and non-zero.
	if tid, ok := rec["tenant_id"].(float64); !ok || tid == 0 {
		t.Fatalf("tenant_id missing/zero on authed request: %v", rec)
	}
	if uid, ok := rec["user_id"].(float64); !ok || uid == 0 {
		t.Fatalf("user_id missing/zero on authed request: %v", rec)
	}
}

func TestRequestLoggerNoSecretsOnLogin(t *testing.T) {
	read := captureLogs(t)
	srv, _, _ := newLoggingServer(t)

	c := jarClient(t)
	// Wrong password drives the warn("failed login attempt") path.
	resp := login(t, c, srv.URL, "o@x.com", "hunter2-supersecret")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login: want 401 got %d", resp.StatusCode)
	}

	recs := read()
	// The login request must have been logged...
	rec := findRequestRecord(t, recs, "/api/auth/login")
	if rec["status"].(float64) != http.StatusUnauthorized {
		t.Fatalf("login record status: %v", rec)
	}
	// ...but NO record may contain the password, a session token, or PII fields.
	for _, r := range recs {
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		blob := strings.ToLower(string(raw))
		if strings.Contains(blob, "hunter2-supersecret") {
			t.Fatalf("password leaked into logs: %s", raw)
		}
		for _, banned := range []string{"password", "password_hash", "token", "ndis_number"} {
			if _, ok := r[banned]; ok {
				t.Fatalf("banned field %q present in log record: %s", banned, raw)
			}
		}
	}
}

func TestLoggerFromFallsBackToDefault(t *testing.T) {
	if LoggerFrom(t.Context()) == nil {
		t.Fatal("LoggerFrom returned nil on bare context")
	}
}
