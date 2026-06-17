package reqctx

import (
	"context"
	"testing"
)

// TestWithUserThenUserFrom verifies the round trip: an attached user id is read
// back with ok = true.
func TestWithUserThenUserFrom(t *testing.T) {
	ctx := WithUser(context.Background(), 99)
	id, ok := UserFrom(ctx)
	if !ok {
		t.Fatal("UserFrom: ok = false, want true")
	}
	if id != 99 {
		t.Fatalf("UserFrom: id = %d, want 99", id)
	}
}

// TestUserFromMissing verifies a bare context reports no user (id 0, ok false).
func TestUserFromMissing(t *testing.T) {
	id, ok := UserFrom(context.Background())
	if ok {
		t.Fatal("UserFrom on bare ctx: ok = true, want false")
	}
	if id != 0 {
		t.Fatalf("UserFrom on bare ctx: id = %d, want 0", id)
	}
}

// TestWithUserOverwrites verifies a later WithUser shadows the earlier value.
func TestWithUserOverwrites(t *testing.T) {
	ctx := WithUser(context.Background(), 1)
	ctx = WithUser(ctx, 2)
	id, ok := UserFrom(ctx)
	if !ok {
		t.Fatal("UserFrom after overwrite: ok = false, want true")
	}
	if id != 2 {
		t.Fatalf("UserFrom after overwrite: id = %d, want 2", id)
	}
}

// TestTenantAndUserCoexist verifies tenant and user use distinct context keys
// and do not clobber each other.
func TestTenantAndUserCoexist(t *testing.T) {
	ctx := WithTenant(context.Background(), 10)
	ctx = WithUser(ctx, 20)
	tid, tok := TenantFrom(ctx)
	uid, uok := UserFrom(ctx)
	if !tok || tid != 10 {
		t.Fatalf("TenantFrom: id=%d ok=%v, want 10/true", tid, tok)
	}
	if !uok || uid != 20 {
		t.Fatalf("UserFrom: id=%d ok=%v, want 20/true", uid, uok)
	}
}

// wrongType is a non-int64 value used to exercise the type-assertion-failure
// branch of TenantFrom / UserFrom.
type wrongType struct{ x int }

// TestTenantFromWrongType verifies that a value of the wrong concrete type
// stored under the tenant key yields (0, false) rather than panicking.
func TestTenantFromWrongType(t *testing.T) {
	// context.WithValue is used directly because WithTenant only accepts int64;
	// we deliberately plant a wrong-typed value to hit the !ok assertion path.
	ctx := context.WithValue(context.Background(), tenantKey, wrongType{x: 1})
	id, ok := TenantFrom(ctx)
	if ok {
		t.Fatal("TenantFrom with wrong type: ok = true, want false")
	}
	if id != 0 {
		t.Fatalf("TenantFrom with wrong type: id = %d, want 0", id)
	}
}

// TestUserFromWrongType is the user-key analogue of TestTenantFromWrongType.
func TestUserFromWrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), userKey, "not-an-int64")
	id, ok := UserFrom(ctx)
	if ok {
		t.Fatal("UserFrom with wrong type: ok = true, want false")
	}
	if id != 0 {
		t.Fatalf("UserFrom with wrong type: id = %d, want 0", id)
	}
}
