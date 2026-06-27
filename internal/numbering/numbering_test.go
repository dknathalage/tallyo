package numbering

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

// setup opens a fresh DB with a per-tenant numbered table that mirrors the real
// invoices uniqueness constraint UNIQUE(tenant_id, number). The allocation logic
// under test (the tenant-scoped MAX read + Format + WithRetry) is exactly what
// the repositories run.
func setup(t *testing.T) *sql.DB {
	t.Helper()
	conn := appdb.OpenTestDB(t)
	if _, err := conn.Exec(`CREATE TABLE doc_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tenant_id INTEGER NOT NULL,
		number TEXT NOT NULL,
		UNIQUE(tenant_id, number))`); err != nil {
		t.Fatalf("create: %v", err)
	}
	return conn
}

const prefix = "INV-"

// nextForTenant reads the current per-tenant max suffix and Formats the next
// number — the same two-step allocation the repos perform.
func nextForTenant(ctx context.Context, tx *sql.Tx, tenantID int64) (string, error) {
	var max int64
	row := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(CAST(substr(number, ?) AS INTEGER)), 0) FROM doc_test WHERE tenant_id = ? AND number LIKE ?`,
		int64(len(prefix)+1), tenantID, prefix+"%")
	if err := row.Scan(&max); err != nil {
		return "", err
	}
	return Format(prefix, max), nil
}

// allocate runs one numbering-retried create for a tenant.
func allocate(ctx context.Context, conn *sql.DB, tenantID int64) error {
	return WithRetry(ctx, 10, func() error {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		n, err := nextForTenant(ctx, tx, tenantID)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO doc_test (tenant_id, number) VALUES (?, ?)`, tenantID, n); err != nil {
			return err
		}
		return tx.Commit()
	})
}

func TestFormatStartsAtOne(t *testing.T) {
	if got := Format("INV-", 0); got != "INV-0001" {
		t.Fatalf("got %q want INV-0001", got)
	}
	if got := Format("EST-", 41); got != "EST-0042" {
		t.Fatalf("got %q want EST-0042", got)
	}
}

func TestSequentialPerTenant(t *testing.T) {
	conn := setup(t)
	ctx := context.Background()
	for _, want := range []string{"INV-0001", "INV-0002", "INV-0003"} {
		tx, _ := conn.BeginTx(ctx, nil)
		n, err := nextForTenant(ctx, tx, 1)
		if err != nil {
			t.Fatalf("next: %v", err)
		}
		if n != want {
			t.Fatalf("got %q want %q", n, want)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO doc_test (tenant_id, number) VALUES (?, ?)`, 1, n); err != nil {
			t.Fatalf("insert: %v", err)
		}
		_ = tx.Commit()
	}
}

// TestConcurrentCreateNoCollisionWithinTenant verifies that concurrent creators
// within ONE tenant each get a unique number (no duplicates) under -race.
func TestConcurrentCreateNoCollisionWithinTenant(t *testing.T) {
	conn := setup(t)
	ctx := context.Background()
	const workers = 16
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- allocate(ctx, conn, 1)
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("worker: %v", e)
		}
	}
	var distinct, total int
	_ = conn.QueryRow(`SELECT COUNT(DISTINCT number), COUNT(*) FROM doc_test WHERE tenant_id = 1`).Scan(&distinct, &total)
	if total != workers || distinct != workers {
		t.Fatalf("tenant 1: distinct=%d total=%d want both %d", distinct, total, workers)
	}
}

// TestTenantsNumberIndependently verifies two tenants number independently:
// concurrent creators across two tenants each produce the same sequence
// (both tenants start at INV-0001), proving per-tenant scoping.
func TestTenantsNumberIndependently(t *testing.T) {
	conn := setup(t)
	ctx := context.Background()
	const perTenant = 10
	var wg sync.WaitGroup
	errs := make(chan error, perTenant*2)
	for _, tenant := range []int64{1, 2} {
		for i := 0; i < perTenant; i++ {
			wg.Add(1)
			go func(tid int64) {
				defer wg.Done()
				errs <- allocate(ctx, conn, tid)
			}(tenant)
		}
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatalf("worker: %v", e)
		}
	}
	// Each tenant must hold exactly the sequence INV-0001..INV-0010.
	for _, tenant := range []int64{1, 2} {
		for n := 1; n <= perTenant; n++ {
			num := fmt.Sprintf("INV-%04d", n)
			var c int
			_ = conn.QueryRow(`SELECT COUNT(*) FROM doc_test WHERE tenant_id = ? AND number = ?`, tenant, num).Scan(&c)
			if c != 1 {
				t.Fatalf("tenant %d missing/duplicate %s (count=%d)", tenant, num, c)
			}
		}
	}
}
