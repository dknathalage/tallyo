package repository

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newTestRepo(t *testing.T) *BusinessProfileRepo {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewBusinessProfile(conn)
}

func TestSaveThenGet(t *testing.T) {
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "r.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	repo := NewBusinessProfile(conn)
	ctx := context.Background()

	if err := repo.Save(ctx, BusinessProfileInput{Name: "Acme", Email: "a@b.com"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "Acme" {
		t.Fatalf("Get = %+v, want Name=Acme", got)
	}
	if got.Email != "a@b.com" {
		t.Fatalf("Email = %q, want a@b.com", got.Email)
	}
	if got.DefaultCurrency != "USD" {
		t.Fatalf("DefaultCurrency = %q, want USD (default)", got.DefaultCurrency)
	}

	var n int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='business_profile'",
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("audit rows = %d, want 1", n)
	}
}

func TestGetReturnsNilWhenEmpty(t *testing.T) {
	repo := newTestRepo(t)
	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get = %+v, want nil", got)
	}
}

func TestSaveRejectsEmptyName(t *testing.T) {
	repo := newTestRepo(t)
	if err := repo.Save(context.Background(), BusinessProfileInput{Name: ""}); err == nil {
		t.Fatalf("Save with empty name: want error, got nil")
	}

	var n int
	if err := repo.db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='business_profile'",
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 0 {
		t.Fatalf("audit rows = %d, want 0 (no row on rejected save)", n)
	}
}

func TestSavePreservesUuidAcrossUpdate(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.Save(ctx, BusinessProfileInput{Name: "First"}); err != nil {
		t.Fatalf("Save #1: %v", err)
	}

	var firstUuid string
	if err := repo.db.QueryRow("SELECT uuid FROM business_profile WHERE id=1").Scan(&firstUuid); err != nil {
		t.Fatalf("read uuid #1: %v", err)
	}
	if firstUuid == "" {
		t.Fatalf("first uuid is empty")
	}

	if err := repo.Save(ctx, BusinessProfileInput{Name: "Second"}); err != nil {
		t.Fatalf("Save #2: %v", err)
	}

	var secondUuid string
	if err := repo.db.QueryRow("SELECT uuid FROM business_profile WHERE id=1").Scan(&secondUuid); err != nil {
		t.Fatalf("read uuid #2: %v", err)
	}
	if secondUuid != firstUuid {
		t.Fatalf("uuid changed: %q -> %q, want unchanged", firstUuid, secondUuid)
	}

	var rows int
	if err := repo.db.QueryRow("SELECT COUNT(*) FROM business_profile").Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("business_profile rows = %d, want 1", rows)
	}
}
