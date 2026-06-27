package reqctx

import (
	"context"
	"testing"
)

func TestWithEntitledRoundTrip(t *testing.T) {
	ctx := WithEntitled(context.Background(), true)
	got, ok := EntitledFrom(ctx)
	if !ok || !got {
		t.Fatalf("EntitledFrom = (%v,%v), want (true,true)", got, ok)
	}
	ctx = WithEntitled(context.Background(), false)
	got, ok = EntitledFrom(ctx)
	if !ok || got {
		t.Fatalf("EntitledFrom = (%v,%v), want (false,true)", got, ok)
	}
}

func TestEntitledFromMissing(t *testing.T) {
	got, ok := EntitledFrom(context.Background())
	if ok || got {
		t.Fatalf("EntitledFrom on bare ctx = (%v,%v), want (false,false)", got, ok)
	}
}
