package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/dknathalage/tallyo/internal/auth"
	"github.com/dknathalage/tallyo/internal/reqctx"
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

// RequireAuth requires a valid session whose userID maps to an existing user.
// The user is re-checked against the store on every request so deleting a user
// invalidates their session immediately. Nil dependencies are programmer errors.
func RequireAuth(sm *scs.SessionManager, users *auth.UsersRepo) func(http.Handler) http.Handler {
	if sm == nil || users == nil {
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
			// tenant-scoped GetByID is filtered to the session's tenant.
			ctx := reqctx.WithTenant(r.Context(), int64(tenantID))
			// Enrich the request-scoped logger so every line for this request
			// (including the final request summary) carries tenant_id/user_id.
			EnrichLogger(ctx, func(l *slog.Logger) *slog.Logger {
				return l.With(slog.Int("tenant_id", tenantID), slog.Int("user_id", id))
			})
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

// UserFrom returns the authenticated user stored on the request context, or nil.
func UserFrom(ctx context.Context) *auth.User {
	u, _ := ctx.Value(userCtxKey).(*auth.User)
	return u
}
