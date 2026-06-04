package auth

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func TestSessionRoundTripAgainstMigratedDB(t *testing.T) {
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	sm := NewSessionManager(conn, false)

	mux := http.NewServeMux()
	mux.HandleFunc("/put", func(w http.ResponseWriter, r *http.Request) {
		sm.Put(r.Context(), "uid", 42)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		v := sm.GetInt(r.Context(), "uid")
		if v != 42 {
			http.Error(w, "missing", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(sm.LoadAndSave(mux))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// store a value (sets session cookie, persists to sessions table)
	resp, err := client.Get(srv.URL + "/put")
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("put status %d", resp.StatusCode)
	}

	// the session row must exist in the migrated table
	var rows int
	if err := conn.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&rows); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if rows != 1 {
		t.Fatalf("sessions rows=%d want 1", rows)
	}

	// retrieve it on a second request with the cookie
	resp2, err := client.Get(srv.URL + "/get")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != 200 {
		t.Fatalf("get status %d (session not persisted/loaded)", resp2.StatusCode)
	}

	_ = context.Background
}
