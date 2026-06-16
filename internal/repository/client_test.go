package repository

import (
	"context"
	"testing"
)

func TestParticipantCreateGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, ParticipantInput{
		Name: "Jane", NDISNumber: "430000001", PlanStart: "2026-01-01", PlanEnd: "2026-12-31", MgmtType: "self",
		Email: "j@x.com",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p == nil || p.ID == 0 || p.Name != "Jane" || p.NDISNumber != "430000001" || p.MgmtType != "self" {
		t.Fatalf("Create = %+v", p)
	}
	got, err := repo.Get(ctx, tid, p.ID)
	if err != nil || got == nil || got.PlanEnd != "2026-12-31" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}
}

func TestParticipantDefaultMgmtType(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	p, err := NewParticipants(conn).Create(context.Background(), tid, ParticipantInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.MgmtType != "plan" {
		t.Fatalf("default mgmtType = %q, want plan", p.MgmtType)
	}
}

func TestParticipantWithPlanManagerName(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	ctx := context.Background()
	pm, err := NewPlanManagers(conn).Create(ctx, tid, PlanManagerInput{Name: "PM Co"})
	if err != nil {
		t.Fatalf("Create PM: %v", err)
	}
	repo := NewParticipants(conn)
	p, err := repo.Create(ctx, tid, ParticipantInput{Name: "Jane", MgmtType: "plan", PlanManagerID: &pm.ID})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.PlanManagerName != "PM Co" {
		t.Fatalf("PlanManagerName = %q, want PM Co", p.PlanManagerName)
	}
}

func TestParticipantSearch(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Alice", NDISNumber: "111"}); err != nil {
		t.Fatalf("Create Alice: %v", err)
	}
	if _, err := repo.Create(ctx, tid, ParticipantInput{Name: "Bob", NDISNumber: "222"}); err != nil {
		t.Fatalf("Create Bob: %v", err)
	}
	// match on ndis number
	res, err := repo.List(ctx, tid, "222")
	if err != nil || len(res) != 1 || res[0].Name != "Bob" {
		t.Fatalf("search ndis = %+v err=%v", res, err)
	}
}

func TestParticipantUpdateDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewParticipants(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, tid, ParticipantInput{Name: "Jane"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, p.ID, ParticipantInput{Name: "Janet", MgmtType: "self"})
	if err != nil || up == nil || up.Name != "Janet" || up.MgmtType != "self" {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if err := repo.Delete(ctx, tid, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := repo.Get(ctx, tid, p.ID); got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestParticipantTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewParticipants(conn)
	ctx := context.Background()

	p, err := repo.Create(ctx, a, ParticipantInput{Name: "Tenant A Person"})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's participant.
	if got, _ := repo.Get(ctx, b, p.ID); got != nil {
		t.Fatalf("tenant B read tenant A's participant: %+v", got)
	}
	if list, _ := repo.List(ctx, b, ""); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
	// Tenant B's update of A's row must not affect it (no rows match → nil).
	if got, _ := repo.Update(ctx, b, p.ID, ParticipantInput{Name: "Hijack"}); got != nil {
		t.Fatalf("tenant B updated tenant A's participant: %+v", got)
	}
	stillA, _ := repo.Get(ctx, a, p.ID)
	if stillA == nil || stillA.Name != "Tenant A Person" {
		t.Fatalf("tenant A's participant was mutated cross-tenant: %+v", stillA)
	}
}
