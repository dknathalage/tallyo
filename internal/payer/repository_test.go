package payer

import (
	"context"
	"testing"
)

func TestPayerCreateGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPayers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, tid, PayerInput{Name: "Acme PM", Email: "a@b.com", Phone: "1", Address: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pm == nil || pm.ID == "" || pm.Name != "Acme PM" || pm.Metadata != "{}" {
		t.Fatalf("Create = %+v", pm)
	}
	got, err := repo.Get(ctx, tid, pm.ID)
	if err != nil || got == nil || got.Name != "Acme PM" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
}

func TestPayerRejectsEmptyName(t *testing.T) {
	// Required-field validation moved from the repository to the service/input
	// (PayerInput.Validate), so assert it there — the repo now trusts its input.
	if err := (PayerInput{Name: ""}).Validate(); err == nil {
		t.Fatal("Validate empty name: want error, got nil")
	}
}

func TestPayerListOrderedAndSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPayers(conn)
	ctx := context.Background()

	for _, n := range []string{"Beta", "Alpha", "Gamma"} {
		if _, err := repo.Create(ctx, tid, PayerInput{Name: n}); err != nil {
			t.Fatalf("Create %s: %v", n, err)
		}
	}
	list, err := repo.List(ctx, tid, "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Alpha", "Beta", "Gamma"}
	for i := range want {
		if list[i].Name != want[i] {
			t.Fatalf("list[%d] = %q, want %q", i, list[i].Name, want[i])
		}
	}

	res, err := repo.List(ctx, tid, "lph")
	if err != nil || len(res) != 1 || res[0].Name != "Alpha" {
		t.Fatalf("search = %+v err=%v", res, err)
	}
}

func TestPayerUpdateDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPayers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, tid, PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, pm.ID, PayerInput{Name: "Acme2", Email: "n@x.com"})
	if err != nil || up == nil || up.Name != "Acme2" || up.Email != "n@x.com" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if err := repo.Delete(ctx, tid, pm.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, pm.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestPayerBulkDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPayers(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, PayerInput{Name: "A"})
	b, _ := repo.Create(ctx, tid, PayerInput{Name: "B"})
	if err := repo.BulkDelete(ctx, tid, []string{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if list, _ := repo.List(ctx, tid, ""); len(list) != 0 {
		t.Fatalf("List len = %d after bulk delete, want 0", len(list))
	}
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete(nil): %v", err)
	}
}

func TestPayerTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewPayers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, a, PayerInput{Name: "A PM"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if got, _ := repo.Get(ctx, b, pm.ID); got != nil {
		t.Fatalf("tenant B read tenant A's payer: %+v", got)
	}
	if list, _ := repo.List(ctx, b, ""); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}

func TestPayerAuditCreate(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPayers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, tid, PayerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var n int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='payer' AND action='create' AND entity_id=?",
		pm.ID,
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("create audit rows = %d, want 1", n)
	}
}
