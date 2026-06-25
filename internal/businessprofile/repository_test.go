package businessprofile

import (
	"context"
	"testing"
)

func TestBusinessProfileSaveThenGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "Acme")
	repo := NewBusinessProfile(conn)
	ctx := context.Background()

	if err := repo.Save(ctx, tid, BusinessProfileInput{Name: "Acme", Email: "a@b.com"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx, tid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "Acme" || got.Email != "a@b.com" {
		t.Fatalf("Get = %+v, want Acme/a@b.com", got)
	}
}

func TestBusinessProfileDefaults(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	repo := NewBusinessProfile(conn)
	ctx := context.Background()

	if err := repo.Save(ctx, tid, BusinessProfileInput{Name: "X"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx, tid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DefaultCurrency != "AUD" {
		t.Fatalf("default currency = %q, want AUD", got.DefaultCurrency)
	}
}

func TestBusinessProfileGetMissing(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	got, err := NewBusinessProfile(conn).Get(context.Background(), tid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get on empty = %+v, want nil", got)
	}
}

func TestBusinessProfileRejectsEmptyName(t *testing.T) {
	// Required-field validation moved from the repository to the service/input
	// (BusinessProfileInput.Validate), so assert it there — the repo now trusts
	// its input.
	if err := (BusinessProfileInput{Name: ""}).Validate(); err == nil {
		t.Fatal("Validate empty name: want error, got nil")
	}
}

func TestBusinessProfileTenantIsolation(t *testing.T) {
	conn := newTestDB(t)
	a := seedTenant(t, conn, "A")
	b := seedTenant(t, conn, "B")
	repo := NewBusinessProfile(conn)
	ctx := context.Background()

	if err := repo.Save(ctx, a, BusinessProfileInput{Name: "Tenant A Co"}); err != nil {
		t.Fatalf("Save A: %v", err)
	}
	// Tenant B has no profile yet; must not see tenant A's.
	got, err := repo.Get(ctx, b)
	if err != nil {
		t.Fatalf("Get B: %v", err)
	}
	if got != nil {
		t.Fatalf("tenant B saw tenant A's profile: %+v", got)
	}
}
