package tenantdb

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
	"github.com/dknathalage/tallyo/internal/reqctx"
)

func newReg(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	control, err := appdb.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatalf("open control: %v", err)
	}
	if err := appdb.MigrateControl(control); err != nil {
		t.Fatalf("migrate control: %v", err)
	}
	reg := New(control, dir)
	t.Cleanup(func() { reg.Close() })
	return reg
}

func TestForTenantID_OpensMigratesCaches(t *testing.T) {
	reg := newReg(t)

	db1, err := reg.ForTenantID(1)
	if err != nil {
		t.Fatalf("ForTenantID(1): %v", err)
	}
	// Migrated: a tenant table must exist and be queryable.
	if _, err := db1.Exec(`INSERT INTO participants (uuid, tenant_id, name, created_at, updated_at)
		VALUES ('u1', 1, 'A', '2026-01-01', '2026-01-01')`); err != nil {
		t.Fatalf("insert into migrated tenant db: %v", err)
	}

	// Cache: same id returns the SAME handle.
	db1b, err := reg.ForTenantID(1)
	if err != nil {
		t.Fatalf("ForTenantID(1) again: %v", err)
	}
	if db1 != db1b {
		t.Fatalf("expected cached handle for tenant 1")
	}

	// Isolation: a different tenant is a different handle, and does not see
	// tenant 1's row (separate file).
	db2, err := reg.ForTenantID(2)
	if err != nil {
		t.Fatalf("ForTenantID(2): %v", err)
	}
	if db1 == db2 {
		t.Fatalf("tenant 2 must get a distinct handle")
	}
	var n int
	if err := db2.QueryRow(`SELECT COUNT(*) FROM participants`).Scan(&n); err != nil {
		t.Fatalf("count tenant 2 participants: %v", err)
	}
	if n != 0 {
		t.Fatalf("tenant 2 leaked %d rows from tenant 1", n)
	}
}

func TestForTenant_FromContext(t *testing.T) {
	reg := newReg(t)
	ctx := reqctx.WithTenant(context.Background(), 7)
	db, err := reg.ForTenant(ctx)
	if err != nil {
		t.Fatalf("ForTenant: %v", err)
	}
	again, _ := reg.ForTenantID(7)
	if db != again {
		t.Fatalf("ForTenant and ForTenantID(7) must resolve the same handle")
	}

	// No tenant in context is an error, not a panic.
	if _, err := reg.ForTenant(context.Background()); err == nil {
		t.Fatalf("expected error when no tenant in context")
	}
}

func TestSweep_KeepsFreshHandles(t *testing.T) {
	reg := newReg(t)
	if _, err := reg.ForTenantID(1); err != nil {
		t.Fatalf("open: %v", err)
	}
	// Freshly used → idle TTL not elapsed → Sweep closes nothing.
	if closed := reg.Sweep(); closed != 0 {
		t.Fatalf("Sweep closed %d fresh handles, want 0", closed)
	}
}
