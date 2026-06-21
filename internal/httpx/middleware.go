package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/reqctx"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
		requestID := uuid.NewString()
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

// RequireAuth requires a valid session whose userID maps to an existing user
// whose tenant is not suspended. The user is re-checked against the store on
// every request so deleting a user (or suspending their tenant) invalidates
// their session immediately. Nil dependencies are programmer errors.
func RequireAuth(sm *scs.SessionManager, users *auth.UsersRepo, tenants *auth.TenantsRepo) func(http.Handler) http.Handler {
	if sm == nil || users == nil || tenants == nil {
		panic("RequireAuth: nil dep")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := sm.GetInt(r.Context(), "userID")
			tenantID := sm.GetInt(r.Context(), "tenantID")
			if id == 0 || tenantID == 0 {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			// Attach the tenant to the context BEFORE the user re-check so the
			// tenant-scoped GetByID is filtered to the session's tenant. Attach
			// the acting user id too so audited mutations record who acted.
			ctx := reqctx.WithTenant(r.Context(), int64(tenantID))
			ctx = reqctx.WithUser(ctx, int64(id))
			// Enrich the request-scoped logger so every line for this request
			// (including the final request summary) carries tenant_id/user_id.
			EnrichLogger(ctx, func(l *slog.Logger) *slog.Logger {
				return l.With(slog.Int("tenant_id", tenantID), slog.Int("user_id", id))
			})
			// Suspended-tenant guard: reject existing sessions whose tenant was
			// suspended after they logged in (spec §3.1).
			status, ok, err := tenants.Status(ctx, int64(tenantID))
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if !ok || status == auth.StatusSuspended {
				if derr := sm.Destroy(ctx); derr != nil {
					LoggerFrom(ctx).Error("destroy session for suspended tenant", slog.Any("error", derr))
				}
				WriteError(w, http.StatusForbidden, "tenant suspended")
				return
			}
			u, err := users.GetByID(ctx, int64(tenantID), int64(id))
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "internal error")
				return
			}
			if u == nil { // user deleted → invalidate session
				if derr := sm.Destroy(ctx); derr != nil {
					LoggerFrom(ctx).Error("destroy session for deleted user", slog.Any("error", derr))
				}
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx = context.WithValue(ctx, userCtxKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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

// MemberLookup resolves the user row (with role) for an email within a tenant,
// returning (nil, nil) when the email is not a member (satisfied by *auth.UsersRepo).
type MemberLookup interface {
	GetByEmail(ctx context.Context, tenantID int64, email string) (*auth.User, error)
}

// RequireSession requires a valid session carrying a userID + email, attaching
// both to the request context. It does NOT resolve a tenant — tenant-agnostic
// authed routes (e.g. /auth/session, /auth/logout) use this alone; tenant-scoped
// routes chain ResolveTenant after it.
func RequireSession(sm *scs.SessionManager) func(http.Handler) http.Handler {
	if sm == nil {
		panic("RequireSession: nil session manager")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := sm.GetInt(r.Context(), "userID")
			email := sm.GetString(r.Context(), "email")
			if id == 0 || email == "" {
				WriteError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			ctx := reqctx.WithUser(r.Context(), int64(id))
			ctx = reqctx.WithEmail(ctx, email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ResolveTenant authorizes the {tenantUUID} URL segment against the session
// email and attaches the resolved tenant + that tenant's user (and role) to the
// context. It is the security core of URL-based multi-tenancy:
//   - email missing → 401 (must be chained after RequireSession)
//   - unknown tenant uuid → 404
//   - suspended tenant → 403
//   - email is not a member of the tenant → 403
//
// On success, downstream handlers read the tenant via reqctx.MustTenant and the
// per-tenant user/role via UserFrom (so role gates reflect THIS tenant's role).
func ResolveTenant(users MemberLookup, tenants TenantLookup) func(http.Handler) http.Handler {
	if users == nil || tenants == nil {
		panic("ResolveTenant: nil dep")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			email, ok := reqctx.EmailFrom(ctx)
			if !ok || email == "" {
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
			u, err := users.GetByEmail(ctx, tenant.ID, email)
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
			tenantID, userID := tenant.ID, u.ID
			EnrichLogger(ctx, func(l *slog.Logger) *slog.Logger {
				return l.With(slog.Int64("tenant_id", tenantID), slog.Int64("user_id", userID))
			})
			ctx = context.WithValue(ctx, userCtxKey, u)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
