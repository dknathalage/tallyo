package gen

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func TestGenRunsAgainstModernc(t *testing.T) {
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	q := New(conn)
	ctx := context.Background()

	if err := q.UpsertBusinessProfile(ctx, UpsertBusinessProfileParams{
		Uuid:            "11111111-1111-1111-1111-111111111111",
		Name:            "Acme",
		Email:           sql.NullString{String: "billing@acme.test", Valid: true},
		Phone:           sql.NullString{String: "555-0100", Valid: true},
		Address:         sql.NullString{String: "1 Acme Way", Valid: true},
		Logo:            sql.NullString{},
		Metadata:        sql.NullString{String: "{}", Valid: true},
		DefaultCurrency: sql.NullString{String: "USD", Valid: true},
		CreatedAt:       sql.NullString{String: "2026-06-04T00:00:00Z", Valid: true},
		UpdatedAt:       sql.NullString{String: "2026-06-04T00:00:00Z", Valid: true},
	}); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	row, err := q.GetBusinessProfile(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if row.ID != 1 {
		t.Fatalf("ID = %d, want 1", row.ID)
	}
	if row.Name != "Acme" {
		t.Fatalf("Name = %q, want %q", row.Name, "Acme")
	}
	if !row.DefaultCurrency.Valid || row.DefaultCurrency.String != "USD" {
		t.Fatalf("DefaultCurrency = %#v, want USD", row.DefaultCurrency)
	}
	if !row.Email.Valid || row.Email.String != "billing@acme.test" {
		t.Fatalf("Email = %#v, want billing@acme.test", row.Email)
	}

	// Verify ON CONFLICT update path: same id=1, changed fields.
	if err := q.UpsertBusinessProfile(ctx, UpsertBusinessProfileParams{
		Uuid:            "22222222-2222-2222-2222-222222222222",
		Name:            "Acme Renamed",
		Email:           sql.NullString{String: "ops@acme.test", Valid: true},
		Phone:           sql.NullString{},
		Address:         sql.NullString{},
		Logo:            sql.NullString{},
		Metadata:        sql.NullString{String: "{}", Valid: true},
		DefaultCurrency: sql.NullString{String: "EUR", Valid: true},
		CreatedAt:       sql.NullString{String: "2026-06-04T00:00:00Z", Valid: true},
		UpdatedAt:       sql.NullString{String: "2026-06-05T00:00:00Z", Valid: true},
	}); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	row, err = q.GetBusinessProfile(ctx)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if row.Name != "Acme Renamed" {
		t.Fatalf("Name after update = %q, want %q", row.Name, "Acme Renamed")
	}
	if !row.DefaultCurrency.Valid || row.DefaultCurrency.String != "EUR" {
		t.Fatalf("DefaultCurrency after update = %#v, want EUR", row.DefaultCurrency)
	}
}
