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

	tenant, err := q.CreateTenant(ctx, CreateTenantParams{
		Uuid:      "00000000-0000-0000-0000-000000000001",
		Name:      "Acme Tenant",
		Status:    "active",
		CreatedAt: "2026-06-04T00:00:00Z",
		UpdatedAt: "2026-06-04T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	if err := q.UpsertBusinessProfile(ctx, UpsertBusinessProfileParams{
		TenantID:        tenant.ID,
		Uuid:            "11111111-1111-1111-1111-111111111111",
		Name:            "Acme",
		Abn:             sql.NullString{String: "12345678901", Valid: true},
		Email:           sql.NullString{String: "billing@acme.test", Valid: true},
		Phone:           sql.NullString{String: "555-0100", Valid: true},
		Address:         sql.NullString{String: "1 Acme Way", Valid: true},
		Zone:            "national",
		Logo:            sql.NullString{},
		Metadata:        sql.NullString{String: "{}", Valid: true},
		DefaultCurrency: sql.NullString{String: "AUD", Valid: true},
		CreatedAt:       "2026-06-04T00:00:00Z",
		UpdatedAt:       "2026-06-04T00:00:00Z",
	}); err != nil {
		t.Fatalf("Upsert (insert): %v", err)
	}

	row, err := q.GetBusinessProfile(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if row.TenantID != tenant.ID {
		t.Fatalf("TenantID = %d, want %d", row.TenantID, tenant.ID)
	}
	if row.Name != "Acme" {
		t.Fatalf("Name = %q, want %q", row.Name, "Acme")
	}
	if row.Zone != "national" {
		t.Fatalf("Zone = %q, want %q", row.Zone, "national")
	}
	if !row.Email.Valid || row.Email.String != "billing@acme.test" {
		t.Fatalf("Email = %#v, want billing@acme.test", row.Email)
	}

	// Verify ON CONFLICT(tenant_id) update path: same tenant, changed fields.
	if err := q.UpsertBusinessProfile(ctx, UpsertBusinessProfileParams{
		TenantID:        tenant.ID,
		Uuid:            "22222222-2222-2222-2222-222222222222",
		Name:            "Acme Renamed",
		Abn:             sql.NullString{},
		Email:           sql.NullString{String: "ops@acme.test", Valid: true},
		Phone:           sql.NullString{},
		Address:         sql.NullString{},
		Zone:            "remote",
		Logo:            sql.NullString{},
		Metadata:        sql.NullString{String: "{}", Valid: true},
		DefaultCurrency: sql.NullString{String: "AUD", Valid: true},
		CreatedAt:       "2026-06-04T00:00:00Z",
		UpdatedAt:       "2026-06-05T00:00:00Z",
	}); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	row, err = q.GetBusinessProfile(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if row.Name != "Acme Renamed" {
		t.Fatalf("Name after update = %q, want %q", row.Name, "Acme Renamed")
	}
	if row.Zone != "remote" {
		t.Fatalf("Zone after update = %q, want %q", row.Zone, "remote")
	}
}
