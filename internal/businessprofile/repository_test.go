package businessprofile

import (
	"context"
	"testing"
)

func TestBusinessProfileSaveThenGet(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "Acme NDIS")
	repo := NewBusinessProfile(conn)
	ctx := context.Background()

	if err := repo.Save(ctx, tid, BusinessProfileInput{Name: "Acme", Email: "a@b.com", Zone: "remote"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx, tid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil")
	}
	if got.Name != "Acme" || got.Email != "a@b.com" || got.Zone != "remote" {
		t.Fatalf("Get = %+v, want Acme/a@b.com/remote", got)
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
	// Empty zone persists as "" (generic, non-NDIS tenant) — NOT coerced to national.
	if got.Zone != "" {
		t.Fatalf("default zone = %q, want \"\" (generic)", got.Zone)
	}
	if got.DefaultCurrency != "AUD" {
		t.Fatalf("default currency = %q, want AUD", got.DefaultCurrency)
	}
}

// TestBusinessProfileEmptyZonePersists confirms an explicitly empty zone is
// stored as "" (generic tenant) and read back as "".
func TestBusinessProfileEmptyZonePersists(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "Generic Co")
	repo := NewBusinessProfile(conn)
	ctx := context.Background()

	if err := repo.Save(ctx, tid, BusinessProfileInput{Name: "Generic Co", Zone: ""}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := repo.Get(ctx, tid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Zone != "" {
		t.Fatalf("zone = %q, want \"\"", got.Zone)
	}
}

// TestBusinessProfileRejectsInvalidZone confirms a non-empty zone outside the
// three NDIS zones is rejected.
func TestBusinessProfileRejectsInvalidZone(t *testing.T) {
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "Bad Co")
	if err := NewBusinessProfile(conn).Save(context.Background(), tid, BusinessProfileInput{Name: "Bad Co", Zone: "foo"}); err == nil {
		t.Fatal("Save invalid zone: want error, got nil")
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
	conn := newTestDB(t)
	tid := seedTenant(t, conn, "T")
	if err := NewBusinessProfile(conn).Save(context.Background(), tid, BusinessProfileInput{Name: ""}); err == nil {
		t.Fatal("Save empty name: want error, got nil")
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
