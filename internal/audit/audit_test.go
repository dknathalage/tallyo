package audit

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func TestLogInsertsRow(t *testing.T) {
	conn := mustDB(t)
	defer conn.Close()

	err := Log(context.Background(), conn, Entry{
		EntityType: "business_profile",
		EntityID:   1,
		Action:     "update",
		Changes:    `{"name":"Acme"}`,
	})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}

	var count int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='business_profile' AND action='update'",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit rows = %d, want 1", count)
	}
}

func TestLogValidatesInputs(t *testing.T) {
	conn := mustDB(t)
	defer conn.Close()

	cases := []struct {
		name  string
		entry Entry
	}{
		{
			name:  "empty entity_type",
			entry: Entry{EntityType: "", Action: "update"},
		},
		{
			name:  "empty action",
			entry: Entry{EntityType: "business_profile", Action: ""},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Log(context.Background(), conn, tc.entry); err == nil {
				t.Fatalf("Log(%+v): expected error, got nil", tc.entry)
			}
		})
	}
}

func mustDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "a.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return conn
}
