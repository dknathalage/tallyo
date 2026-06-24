package taxrate

import (
	"context"
	"testing"
)

func TestTaxRateCreateGetList(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewTaxRates(conn)
	ctx := context.Background()

	gst, err := repo.Create(ctx, tid, TaxRateInput{Name: "GST", Rate: 10, IsDefault: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gst.ID == "" || !gst.IsDefault || gst.Rate != 10 {
		t.Fatalf("Create = %+v", gst)
	}

	got, err := repo.Get(ctx, tid, gst.ID)
	if err != nil || got == nil || got.Name != "GST" {
		t.Fatalf("Get = %+v err=%v", got, err)
	}

	list, err := repo.List(ctx, tid)
	if err != nil || len(list) != 1 {
		t.Fatalf("List len = %d err=%v", len(list), err)
	}
}

func TestTaxRateOnlyOneDefault(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewTaxRates(conn)
	ctx := context.Background()

	if _, err := repo.Create(ctx, tid, TaxRateInput{Name: "A", Rate: 5, IsDefault: true}); err != nil {
		t.Fatalf("Create A: %v", err)
	}
	if _, err := repo.Create(ctx, tid, TaxRateInput{Name: "B", Rate: 10, IsDefault: true}); err != nil {
		t.Fatalf("Create B: %v", err)
	}
	def, err := repo.GetDefault(ctx, tid)
	if err != nil || def == nil || def.Name != "B" {
		t.Fatalf("GetDefault = %+v err=%v, want B", def, err)
	}
}

func TestTaxRateUpdateDelete(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewTaxRates(conn)
	ctx := context.Background()

	tr, err := repo.Create(ctx, tid, TaxRateInput{Name: "GST", Rate: 10})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	up, err := repo.Update(ctx, tid, tr.ID, TaxRateInput{Name: "GST2", Rate: 12})
	if err != nil || up == nil || up.Name != "GST2" || up.Rate != 12 {
		t.Fatalf("Update = %+v err=%v", up, err)
	}
	if err := repo.Delete(ctx, tid, tr.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	got, _ := repo.Get(ctx, tid, tr.ID)
	if got != nil {
		t.Fatalf("row present after delete: %+v", got)
	}
}

func TestTaxRateTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewTaxRates(conn)
	ctx := context.Background()

	tr, err := repo.Create(ctx, a, TaxRateInput{Name: "GST", Rate: 10})
	if err != nil {
		t.Fatalf("Create A: %v", err)
	}
	// Tenant B cannot read tenant A's row.
	got, err := repo.Get(ctx, b, tr.ID)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if got != nil {
		t.Fatalf("tenant B read tenant A's tax rate: %+v", got)
	}
	if list, _ := repo.List(ctx, b); len(list) != 0 {
		t.Fatalf("tenant B List len = %d, want 0", len(list))
	}
}
