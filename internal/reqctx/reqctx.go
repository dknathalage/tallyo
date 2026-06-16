// Package reqctx carries request-scoped values (currently the tenant id)
// through context.Context. It has no dependencies on other internal packages so
// it can be imported anywhere — repositories, services, and HTTP middleware —
// without creating import cycles.
//
// Tenant scoping is the data-layer half of multi-tenant isolation: the auth
// middleware resolves the caller's tenant from the session and stores it here
// with WithTenant; repository callers read it with TenantFrom / MustTenant and
// pass it into every tenant-scoped query.
package reqctx

import "context"

// ctxKey is an unexported type for context keys defined in this package. Using a
// private type prevents collisions with keys defined in other packages, which is
// the idiomatic Go pattern for context values.
type ctxKey int

const (
	// tenantKey is the context key under which the tenant id is stored.
	tenantKey ctxKey = iota
)

// WithTenant returns a copy of ctx that carries the given tenant id. A non-zero
// tenant id is expected; callers that need a value should validate at the
// boundary (this helper does not, so it stays allocation-cheap and total).
func WithTenant(ctx context.Context, tenantID int64) context.Context {
	return context.WithValue(ctx, tenantKey, tenantID)
}

// TenantFrom returns the tenant id stored in ctx and whether one was present.
// ok is false (and the id is 0) when no tenant has been attached.
func TenantFrom(ctx context.Context) (int64, bool) {
	v := ctx.Value(tenantKey)
	if v == nil {
		return 0, false
	}
	id, ok := v.(int64)
	if !ok {
		return 0, false
	}
	return id, true
}

// MustTenant returns the tenant id stored in ctx, panicking if none is present.
//
// A missing tenant at this point is a PROGRAMMER ERROR: it means a tenant-scoped
// code path ran without the auth middleware having attached a tenant (or a test
// forgot to seed one). Per NASA rule 5 we fail loudly on the invariant violation
// rather than silently defaulting to tenant 0 and leaking data across tenants.
func MustTenant(ctx context.Context) int64 {
	id, ok := TenantFrom(ctx)
	if !ok {
		panic("reqctx: tenant id missing from context (programmer error)")
	}
	return id
}
