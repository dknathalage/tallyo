package reqctx

import (
	"context"
	"testing"
)

func TestWithTenantThenTenantFrom(t *testing.T) {
	ctx := WithTenant(context.Background(), "t-42")
	id, ok := TenantFrom(ctx)
	if !ok {
		t.Fatal("TenantFrom: ok = false, want true")
	}
	if id != "t-42" {
		t.Fatalf("TenantFrom: id = %q, want t-42", id)
	}
}

func TestTenantFromMissing(t *testing.T) {
	id, ok := TenantFrom(context.Background())
	if ok {
		t.Fatal("TenantFrom on bare ctx: ok = true, want false")
	}
	if id != "" {
		t.Fatalf("TenantFrom on bare ctx: id = %q, want empty", id)
	}
}

func TestMustTenantPresent(t *testing.T) {
	ctx := WithTenant(context.Background(), "t-7")
	if got := MustTenant(ctx); got != "t-7" {
		t.Fatalf("MustTenant: got %q, want t-7", got)
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
	ctx := WithTenant(context.Background(), "t-1")
	ctx = WithTenant(ctx, "t-2")
	id, ok := TenantFrom(ctx)
	if !ok || id != "t-2" {
		t.Fatalf("TenantFrom after overwrite: id=%q ok=%v, want t-2/true", id, ok)
	}
}
