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
	// userKey is the context key under which the acting user id is stored. The
	// audit layer reads it to stamp every mutation with who performed it.
	userKey
	// emailKey is the context key under which the authenticated email (the
	// durable cross-tenant identity) is stored. ResolveTenant reads it to
	// authorize the URL tenant per request.
	emailKey
	// firebaseUIDKey is the context key under which the verified Firebase uid
	// (the token-stable cross-tenant identity) is stored. RequireAuth sets it
	// from the bearer token; ResolveTenant reads it to authorize the URL tenant.
	firebaseUIDKey
	// entitledKey is the context key under which the tenant's billing entitlement
	// (a bool) is stored. ResolveTenant sets it; RequireSubscription reads it to
	// gate write methods. Absent means "gate disabled" → treated as entitled.
	entitledKey
)

// WithEntitled returns a copy of ctx carrying the tenant's billing entitlement.
// Set by ResolveTenant; read by RequireSubscription.
func WithEntitled(ctx context.Context, entitled bool) context.Context {
	return context.WithValue(ctx, entitledKey, entitled)
}

// EntitledFrom returns the entitlement flag stored in ctx and whether one was
// present. ok is false when no flag was attached (e.g. the billing gate is off);
// callers treat that as entitled.
func EntitledFrom(ctx context.Context) (entitled bool, ok bool) {
	v := ctx.Value(entitledKey)
	if v == nil {
		return false, false
	}
	b, ok := v.(bool)
	if !ok {
		return false, false
	}
	return b, true
}

// WithFirebaseUID returns a copy of ctx carrying the verified Firebase uid. Set
// by RequireAuth from the bearer token; read by ResolveTenant.
func WithFirebaseUID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, firebaseUIDKey, uid)
}

// FirebaseUIDFrom returns the verified Firebase uid stored in ctx and whether
// one was present. ok is false (and the string empty) when no uid was attached.
func FirebaseUIDFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(firebaseUIDKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, ok
}

// WithEmail returns a copy of ctx carrying the authenticated email. Set by the
// session middleware; read by the URL-tenant resolver.
func WithEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, emailKey, email)
}

// EmailFrom returns the authenticated email stored in ctx and whether one was
// present. ok is false (and the string empty) when no email has been attached.
func EmailFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(emailKey)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	return s, ok
}

// WithTenant returns a copy of ctx that carries the given tenant id (a uuid
// string). A non-empty tenant id is expected; callers that need a value should
// validate at the boundary (this helper does not, so it stays allocation-cheap
// and total).
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, tenantKey, tenantID)
}

// TenantFrom returns the tenant id stored in ctx and whether one was present.
// ok is false (and the id is "") when no tenant has been attached.
func TenantFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(tenantKey)
	if v == nil {
		return "", false
	}
	id, ok := v.(string)
	if !ok {
		return "", false
	}
	return id, true
}

// WithUser returns a copy of ctx that carries the acting user's id (a uuid
// string). The auth middleware attaches it after resolving the session so
// audited mutations can record who performed them. An empty/absent user id means
// "no user" (e.g. system sweeps or the pre-auth signup transaction) and is
// recorded as NULL by audit.
func WithUser(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userKey, userID)
}

// UserFrom returns the acting user id stored in ctx and whether one was present.
// ok is false (and the id is "") when no user has been attached.
func UserFrom(ctx context.Context) (string, bool) {
	v := ctx.Value(userKey)
	if v == nil {
		return "", false
	}
	id, ok := v.(string)
	if !ok {
		return "", false
	}
	return id, true
}

// MustTenant returns the tenant id stored in ctx, panicking if none is present.
//
// A missing tenant at this point is a PROGRAMMER ERROR: it means a tenant-scoped
// code path ran without the auth middleware having attached a tenant (or a test
// forgot to seed one). Per NASA rule 5 we fail loudly on the invariant violation
// rather than silently defaulting to tenant 0 and leaking data across tenants.
func MustTenant(ctx context.Context) string {
	id, ok := TenantFrom(ctx)
	if !ok {
		panic("reqctx: tenant id missing from context (programmer error)")
	}
	return id
}
