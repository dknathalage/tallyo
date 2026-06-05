package repository

import (
	"context"
	"path/filepath"
	"testing"

	appdb "github.com/dknathalage/tallyo/internal/db"
)

func newPayersRepo(t *testing.T) *PayersRepo {
	t.Helper()
	conn, err := appdb.Open(filepath.Join(t.TempDir(), "payer.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := appdb.Migrate(conn); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return NewPayers(conn)
}

func TestPayerCreate(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	p, err := repo.Create(ctx, PayerInput{Name: "Acme", Email: "a@b.com", Phone: "1", Address: "x", Metadata: ""})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil {
		t.Fatal("Create returned nil payer")
	}
	if p.ID <= 0 {
		t.Fatalf("ID = %d, want > 0", p.ID)
	}
	if p.Name != "Acme" {
		t.Fatalf("Name = %q, want Acme", p.Name)
	}
	if p.Email != "a@b.com" {
		t.Fatalf("Email = %q, want a@b.com", p.Email)
	}
	if p.Phone != "1" {
		t.Fatalf("Phone = %q, want 1", p.Phone)
	}
	if p.Address != "x" {
		t.Fatalf("Address = %q, want x", p.Address)
	}
	if p.Metadata != "{}" {
		t.Fatalf("Metadata = %q, want {} (default)", p.Metadata)
	}
}

func TestPayerCreateRejectsEmptyName(t *testing.T) {
	repo := newPayersRepo(t)
	if _, err := repo.Create(context.Background(), PayerInput{Name: ""}); err == nil {
		t.Fatal("Create with empty name: want error, got nil")
	}
}

func TestPayerListOrdered(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, PayerInput{Name: "Beta"}); err != nil {
		t.Fatalf("Create Beta: %v", err)
	}
	if _, err := repo.Create(ctx, PayerInput{Name: "Alpha"}); err != nil {
		t.Fatalf("Create Alpha: %v", err)
	}
	if _, err := repo.Create(ctx, PayerInput{Name: "Gamma"}); err != nil {
		t.Fatalf("Create Gamma: %v", err)
	}

	list, err := repo.List(ctx, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Alpha", "Beta", "Gamma"}
	if len(list) != len(want) {
		t.Fatalf("List len = %d, want %d", len(list), len(want))
	}
	for i := range want {
		if list[i].Name != want[i] {
			t.Fatalf("list[%d].Name = %q, want %q", i, list[i].Name, want[i])
		}
	}
}

func TestPayerListSearch(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	if _, err := repo.Create(ctx, PayerInput{Name: "Acme Corp", Email: "z@z.com"}); err != nil {
		t.Fatalf("Create Acme: %v", err)
	}
	if _, err := repo.Create(ctx, PayerInput{Name: "Globex", Email: "g@g.com"}); err != nil {
		t.Fatalf("Create Globex: %v", err)
	}
	// Email match (name does not contain "acm").
	if _, err := repo.Create(ctx, PayerInput{Name: "Zilch", Email: "acm@x.com"}); err != nil {
		t.Fatalf("Create Zilch: %v", err)
	}

	list, err := repo.List(ctx, "acm")
	if err != nil {
		t.Fatalf("List search: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("search len = %d, want 2 (name + email match)", len(list))
	}
	got := map[string]bool{}
	for _, p := range list {
		got[p.Name] = true
	}
	if !got["Acme Corp"] || !got["Zilch"] {
		t.Fatalf("search results = %v, want Acme Corp (name) + Zilch (email)", got)
	}
	if got["Globex"] {
		t.Fatal("Globex matched but should not")
	}
}

func TestPayerGet(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Name != "Acme" {
		t.Fatalf("Get = %+v, want Name=Acme", got)
	}

	missing, err := repo.Get(ctx, 99999)
	if err != nil {
		t.Fatalf("Get missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Get missing = %+v, want nil", missing)
	}
}

func TestPayerUpdate(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, PayerInput{Name: "Acme", Email: "old@x.com", Phone: "1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := repo.Update(ctx, created.ID, PayerInput{Name: "Acme2", Email: "new@x.com", Phone: "2", Address: "y", Metadata: ""})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if updated.Name != "Acme2" || updated.Email != "new@x.com" || updated.Phone != "2" || updated.Address != "y" {
		t.Fatalf("Update = %+v, want Acme2/new@x.com/2/y", updated)
	}
	if updated.Metadata != "{}" {
		t.Fatalf("Update Metadata = %q, want {} (default)", updated.Metadata)
	}

	missing, err := repo.Update(ctx, 99999, PayerInput{Name: "Nope"})
	if err != nil {
		t.Fatalf("Update missing: %v", err)
	}
	if missing != nil {
		t.Fatalf("Update missing = %+v, want nil", missing)
	}
}

func TestPayerDelete(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if got != nil {
		t.Fatalf("row still present after delete: %+v", got)
	}
}

func TestPayerBulkDelete(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	a, err := repo.Create(ctx, PayerInput{Name: "A"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	b, err := repo.Create(ctx, PayerInput{Name: "B"})
	if err != nil {
		t.Fatalf("Create B: %v", err)
	}

	if err := repo.BulkDelete(ctx, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	for _, id := range []int64{a.ID, b.ID} {
		got, err := repo.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get %d after bulk delete: %v", id, err)
		}
		if got != nil {
			t.Fatalf("payer %d still present after bulk delete: %+v", id, got)
		}
	}

	// Empty slice is a no-op, not an error.
	if err := repo.BulkDelete(ctx, nil); err != nil {
		t.Fatalf("BulkDelete(nil): %v", err)
	}
}

func TestPayerAuditCreate(t *testing.T) {
	repo := newPayersRepo(t)
	ctx := context.Background()

	p, err := repo.Create(ctx, PayerInput{Name: "Acme", Email: "a@b.com"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var n int
	if err := repo.db.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='payer' AND action='create' AND entity_id=?",
		p.ID,
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("create audit rows for id=%d = %d, want 1", p.ID, n)
	}
}
