package planmanager

import (
	"context"
	"testing"
)

func TestPlanManagerCreateGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPlanManagers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, tid, PlanManagerInput{Name: "Acme PM", Email: "a@b.com", Phone: "1", Address: "x"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if pm == nil || pm.ID == 0 || pm.Name != "Acme PM" || pm.Metadata != "{}" {
		t.Fatalf("Create = %+v", pm)
	}
	got, err := repo.Get(ctx, tid, pm.UUID)
	if err != nil || got == nil || got.Name != "Acme PM" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
}

func TestPlanManagerRejectsEmptyName(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	if _, err := NewPlanManagers(conn).Create(context.Background(), tid, PlanManagerInput{Name: ""}); err == nil {
		t.Fatal("Create empty name: want error, got nil")
	}
}

func TestPlanManagerListOrderedAndSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPlanManagers(conn)
	ctx := context.Background()

	for _, n := range []string{"Beta", "Alpha", "Gamma"} {
		if _, err := repo.Create(ctx, tid, PlanManagerInput{Name: n}); err != nil {
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

func TestPlanManagerUpdateDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPlanManagers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, tid, PlanManagerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, pm.UUID, PlanManagerInput{Name: "Acme2", Email: "n@x.com"})
	if err != nil || up == nil || up.Name != "Acme2" || up.Email != "n@x.com" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if err := repo.Delete(ctx, tid, pm.UUID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, pm.UUID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestPlanManagerBulkDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPlanManagers(conn)
	ctx := context.Background()

	a, _ := repo.Create(ctx, tid, PlanManagerInput{Name: "A"})
	b, _ := repo.Create(ctx, tid, PlanManagerInput{Name: "B"})
	if err := repo.BulkDelete(ctx, tid, []int64{a.ID, b.ID}); err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if list, _ := repo.List(ctx, tid, ""); len(list) != 0 {
		t.Fatalf("List len = %d after bulk delete, want 0", len(list))
	}
	if err := repo.BulkDelete(ctx, tid, nil); err != nil {
		t.Fatalf("BulkDelete(nil): %v", err)
	}
}

func TestPlanManagerTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewPlanManagers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, a, PlanManagerInput{Name: "A PM"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if got, _ := repo.Get(ctx, b, pm.UUID); got != nil {
		t.Fatalf("tenant B read tenant A's plan manager: %+v", got)
	}
	if list, _ := repo.List(ctx, b, ""); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}

func TestPlanManagerAuditCreate(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewPlanManagers(conn)
	ctx := context.Background()

	pm, err := repo.Create(ctx, tid, PlanManagerInput{Name: "Acme"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	var n int
	if err := conn.QueryRow(
		"SELECT COUNT(*) FROM audit_log WHERE entity_type='plan_manager' AND action='create' AND entity_id=?",
		pm.ID,
	).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 1 {
		t.Fatalf("create audit rows = %d, want 1", n)
	}
}
