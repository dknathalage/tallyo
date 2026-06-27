package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/entitlement"
	"github.com/dknathalage/tallyo/internal/ids"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
)

type ctxKey int

const userCtxKey ctxKey = 0

// Recover turns panics into 500s without crashing the server. The recovered
// value (and stack) is logged at error level via the request-scoped logger so
// the line carries request_id and, when authenticated, tenant_id/user_id.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				LoggerFrom(r.Context()).Error("panic recovered",
					slog.Any("panic", rec),
					slog.String("stack", string(debug.Stack())),
				)
				WriteError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestLogger builds a request-scoped logger (tagged with a unique request_id)
// and stores it in the context, then emits one structured line per request with
// method, path, status, duration_ms, and request_id. Downstream middleware
// (RequireAuth) enriches the same context logger with tenant_id/user_id, so the
// final line carries them when the request is authenticated.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := ids.New()
		base := slog.Default().With(slog.String("request_id", requestID))
		ctx := WithLogger(r.Context(), base)
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r.WithContext(ctx))
		// The logger is held behind a pointer, so any downstream enrichment
		// (RequireAuth adding tenant_id/user_id) is visible here on ctx.
		LoggerFrom(ctx).Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

// statusWriter captures the status code written to the response.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap exposes the wrapped writer so http.ResponseController can reach
// optional interfaces (e.g. http.Flusher) on writers further down the chain.
// Without this, streaming endpoints (SSE) cannot flush through this wrapper.
func (s *statusWriter) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// RequireAuth parses the "Authorization: Bearer <token>" header, verifies it as
// a Firebase ID token, and attaches the verified uid + email to the context. It
// is the stateless replacement for the old scs RequireSession: it does NOT
// resolve a tenant — tenant-agnostic authed routes (e.g. /auth/session) use it
// alone, while tenant-scoped routes chain ResolveTenant after it.
//
// A missing/malformed header or an invalid token yields 401. A nil verifier is a
// programmer error.
func RequireAuth(verifier auth.TokenVerifier) func(http.Handler) http.Handler {
	if verifier == nil {
		panic("RequireAuth: nil verifier")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r)
			if !ok {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			tok, err := verifier.VerifyIDToken(r.Context(), raw)
			if err != nil {
				LoggerFrom(r.Context()).Warn("bearer token rejected")
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if tok.UID == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := reqctx.WithFirebaseUID(r.Context(), tok.UID)
			ctx = reqctx.WithEmail(ctx, tok.Email)
			EnrichLogger(ctx, func(l *slog.Logger) *slog.Logger {
				return l.With(slog.String("firebase_uid", tok.UID))
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the raw token from the Authorization header. ok is false
// when the header is absent or not a well-formed "Bearer <token>".
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// RequireRole gates a route to users holding one of the allowed tenant roles
// (owner | admin | member). It must be chained AFTER RequireAuth, which places
// the authenticated user on the context. A missing user is treated as 401; an
// insufficient role as 403.
func RequireRole(allowed ...string) func(http.Handler) http.Handler {
	if len(allowed) == 0 {
		panic("RequireRole: at least one role is required")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u := UserFrom(r.Context())
			if u == nil {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			for i := range allowed {
				if u.Role == allowed[i] {
					next.ServeHTTP(w, r)
					return
				}
			}
			WriteError(w, http.StatusForbidden, "insufficient role")
		})
	}
}

// RequireSubscription blocks write methods (POST/PUT/PATCH/DELETE) for tenants
// without an entitled subscription, returning 402. Reads (GET/HEAD/OPTIONS)
// always pass — a lapsed tenant keeps read + export access. Chain it AFTER
// ResolveTenant (which sets the entitled flag). Mount it only on the non-billing
// route group so a lapsed tenant can still reach Checkout/Portal to pay.
//
// A missing entitled flag (gate disabled — ResolveTenant never set it) is treated
// as entitled, so this middleware is a no-op when BILLING_ENABLED is off.
func RequireSubscription(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entitled, ok := reqctx.EntitledFrom(r.Context())
		if ok && !entitled {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
				WriteError(w, http.StatusPaymentRequired, "subscription required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePlatformAdmin gates a route to platform admins. is_platform_admin is
// ORTHOGONAL to the tenant role (spec §3.1): it is only checked for the global
// catalogue-admin area (J7 ingest). Must be chained after RequireAuth.
func RequirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := UserFrom(r.Context())
		if u == nil {
			WriteError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !u.IsPlatformAdmin {
			WriteError(w, http.StatusForbidden, "platform admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UserFrom returns the authenticated user stored on the request context, or nil.
func UserFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userCtxKey).(*auth.User)
	return u
}

// TenantLookup resolves a tenant by its public UUID (satisfied by *auth.TenantsRepo).
type TenantLookup interface {
	GetByUUID(ctx context.Context, tenantUUID string) (*auth.Tenant, error)
}

// MemberLookup resolves the user row (with role) for a Firebase uid within a
// tenant, returning (nil, nil) when the uid is not a member (satisfied by
// *auth.UsersRepo).
type MemberLookup interface {
	GetByFirebaseUID(ctx context.Context, tenantID string, firebaseUID string) (*auth.User, error)
}

// ResolveTenant authorizes the {tenantUUID} URL segment against the verified
// Firebase uid and attaches the resolved tenant + that tenant's user (and role)
// to the context. It is the security core of URL-based multi-tenancy:
//   - uid missing → 401 (must be chained after RequireAuth)
//   - unknown tenant uuid → 404
//   - suspended tenant → 403
//   - uid is not a member of the tenant → 403
//
// On success, downstream handlers read the tenant via reqctx.MustTenant and the
// per-tenant user/role via UserFrom (so role gates reflect THIS tenant's role).
func ResolveTenant(users MemberLookup, tenants TenantLookup, billingEnabled bool) func(http.Handler) http.Handler {
	if users == nil || tenants == nil {
		panic("ResolveTenant: nil dep")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			uid, ok := reqctx.FirebaseUIDFrom(ctx)
			if !ok || uid == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			tenantUUID := chi.URLParam(r, "tenantUUID")
			if tenantUUID == "" {
				WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			tenant, err := tenants.GetByUUID(ctx, tenantUUID)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if tenant == nil {
				WriteError(w, http.StatusNotFound, "tenant not found")
				return
			}
			if tenant.Status == auth.StatusSuspended {
				WriteError(w, http.StatusForbidden, "tenant suspended")
				return
			}
			u, err := users.GetByFirebaseUID(ctx, tenant.ID, uid)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if u == nil { // email is not a member of this tenant
				WriteError(w, http.StatusForbidden, "forbidden")
				return
			}
			ctx = reqctx.WithTenant(ctx, tenant.ID)
			ctx = reqctx.WithUser(ctx, u.ID)
			// Entitlement rides along the already-loaded tenant (no extra read).
			// Gate off → always entitled. Read by RequireSubscription.
			entitled := !billingEnabled || entitlement.Entitled(tenant.SubscriptionStatus)
			ctx = reqctx.WithEntitled(ctx, entitled)
			tenantID, userID := tenant.ID, u.ID
			EnrichLogger(ctx, func(l *slog.Logger) *slog.Logger {
				return l.With(slog.String("tenant_id", tenantID), slog.String("user_id", userID))
			})
			ctx = context.WithValue(ctx, userCtxKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
