package app

import (
	"bytes"
	"encoding/json"
	"github.com/dknathalage/tallyo/internal/httpx"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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

// newLoggingServer builds a server with the full httpx.Recover→httpx.RequestLogger→Session
// chain plus an authenticated probe, so the request logger and httpx.RequireAuth
// enrichment are exercised end to end.
func newLoggingServer(t *testing.T) (*httptest.Server, *auth.UsersRepo, string) {
	t.Helper()
	conn := openMigratedDB(t, "log.db")
	users, _, _, tenantUUID := seedTenantOwner(t, conn)
	v := newStubVerifier()
	tenants := auth.NewTenants(conn)

	router := chi.NewRouter()
	router.Use(httpx.Recover)
	router.Use(httpx.RequestLogger)
	router.Route("/api", func(api chi.Router) {
		api.Route("/t/{tenantUUID}", func(pr chi.Router) {
			pr.Use(httpx.RequireAuth(v))
			pr.Use(httpx.ResolveTenant(users, tenants, false))
			pr.Get("/probe", probe200)
		})
	})

	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	return srv, users, tenantUUID
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
	srv, _, uuid := newLoggingServer(t)

	c := loggedInClient(t, srv.URL)
	probePath := "/api/t/" + uuid + "/probe"
	pr := get(t, c, srv.URL+probePath)
	_ = pr.Body.Close()
	if pr.StatusCode != http.StatusOK {
		t.Fatalf("probe: want 200 got %d", pr.StatusCode)
	}

	rec := findRequestRecord(t, read(), probePath)

	for _, field := range []string{"request_id", "method", "path", "status", "duration_ms"} {
		if _, ok := rec[field]; !ok {
			t.Fatalf("request record missing %q: %v", field, rec)
		}
	}
	if rid, _ := rec["request_id"].(string); rid == "" {
		t.Fatalf("request_id empty: %v", rec)
	}
	// Authenticated request → tenant_id and user_id present and non-empty
	// (uuid strings now, not int PKs).
	if tid, ok := rec["tenant_id"].(string); !ok || tid == "" {
		t.Fatalf("tenant_id missing/empty on authed request: %v", rec)
	}
	if uid, ok := rec["user_id"].(string); !ok || uid == "" {
		t.Fatalf("user_id missing/empty on authed request: %v", rec)
	}
}

func TestRequestLoggerNoSecretsOnAuthedRequest(t *testing.T) {
	read := captureLogs(t)
	srv, _, uuid := newLoggingServer(t)

	// A bogus bearer token drives the warn("bearer token rejected") path.
	c := bearerClient("super-secret-bearer-token")
	resp := get(t, c, srv.URL+"/api/t/"+uuid+"/probe")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: want 401 got %d", resp.StatusCode)
	}

	recs := read()
	// NO record may contain the raw bearer token or password/token PII fields.
	for _, r := range recs {
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		blob := strings.ToLower(string(raw))
		if strings.Contains(blob, "super-secret-bearer-token") {
			t.Fatalf("bearer token leaked into logs: %s", raw)
		}
		for _, banned := range []string{"password", "password_hash", "token", "authorization"} {
			if _, ok := r[banned]; ok {
				t.Fatalf("banned field %q present in log record: %s", banned, raw)
			}
		}
	}
}

func TestLoggerFromFallsBackToDefault(t *testing.T) {
	if httpx.LoggerFrom(t.Context()) == nil {
		t.Fatal("httpx.LoggerFrom returned nil on bare context")
	}
}
