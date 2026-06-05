package audit

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func mustWDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "w.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}

func TestWithTxCommitsAndAudits(t *testing.T) {
	conn := mustWDB(t)
	defer conn.Close()
	ctx := context.Background()
	err := WithTx(ctx, conn, Entry{EntityType: "business_profile", EntityID: 1, Action: "update", Changes: Changes(map[string]any{"name": "Acme"})},
		func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, `INSERT INTO business_profile (id, uuid, name) VALUES (1, 'u', 'Acme')`)
			return e
		})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM business_profile`).Scan(&n)
	if n != 1 {
		t.Fatalf("profile rows=%d want 1", n)
	}
	conn.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE entity_type='business_profile'`).Scan(&n)
	if n != 1 {
		t.Fatalf("audit rows=%d want 1", n)
	}
}

func TestWithTxRollsBackOnFnError(t *testing.T) {
	conn := mustWDB(t)
	defer conn.Close()
	ctx := context.Background()
	boom := errors.New("boom")
	err := WithTx(ctx, conn, Entry{EntityType: "business_profile", EntityID: 1, Action: "update"},
		func(tx *sql.Tx) error {
			tx.ExecContext(ctx, `INSERT INTO business_profile (id, uuid, name) VALUES (1, 'u', 'X')`)
			return boom
		})
	if !errors.Is(err, boom) {
		t.Fatalf("want boom, got %v", err)
	}
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM business_profile`).Scan(&n)
	if n != 0 {
		t.Fatalf("profile rows=%d want 0 (rolled back)", n)
	}
	conn.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n)
	if n != 0 {
		t.Fatalf("audit rows=%d want 0 (rolled back)", n)
	}
}

func TestWithTxSkipsAutoLogWhenActionEmpty(t *testing.T) {
	conn := mustWDB(t)
	defer conn.Close()
	ctx := context.Background()
	// Action == "" → WithTx must NOT auto-log; the fn logs manually if it wants.
	err := WithTx(ctx, conn, Entry{EntityType: "x", Action: ""},
		func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, `INSERT INTO business_profile (id, uuid, name) VALUES (1, 'u', 'A')`)
			return e
		})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	var n int
	conn.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&n)
	if n != 0 {
		t.Fatalf("audit rows=%d want 0 (auto-log skipped)", n)
	}
}

func TestChangesProducesJSON(t *testing.T) {
	s := Changes(map[string]any{"name": "Acme", "n": 3})
	if s == "" || s[0] != '{' {
		t.Fatalf("Changes=%q", s)
	}
}
