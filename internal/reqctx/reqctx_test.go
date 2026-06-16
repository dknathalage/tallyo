package reqctx

import (
	"context"
	"testing"
)

func TestWithTenantThenTenantFrom(t *testing.T) {
	ctx := WithTenant(context.Background(), 42)
	id, ok := TenantFrom(ctx)
	if !ok {
		t.Fatal("TenantFrom: ok = false, want true")
	}
	if id != 42 {
		t.Fatalf("TenantFrom: id = %d, want 42", id)
	}
}

func TestTenantFromMissing(t *testing.T) {
	id, ok := TenantFrom(context.Background())
	if ok {
		t.Fatal("TenantFrom on bare ctx: ok = true, want false")
	}
	if id != 0 {
		t.Fatalf("TenantFrom on bare ctx: id = %d, want 0", id)
	}
}

func TestMustTenantPresent(t *testing.T) {
	ctx := WithTenant(context.Background(), 7)
	if got := MustTenant(ctx); got != 7 {
		t.Fatalf("MustTenant: got %d, want 7", got)
	}
}

func TestMustTenantPanicsWhenMissing(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustTenant on bare ctx: expected panic, got none")
		}
	}()
	_ = MustTenant(context.Background())
}

func TestWithTenantOverwrites(t *testing.T) {
	ctx := WithTenant(context.Background(), 1)
	ctx = WithTenant(ctx, 2)
	id, ok := TenantFrom(ctx)
	if !ok || id != 2 {
		t.Fatalf("TenantFrom after overwrite: id=%d ok=%v, want 2/true", id, ok)
	}
}
